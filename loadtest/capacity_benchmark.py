#!/usr/bin/env python3
"""Isolated measured benchmark. Failures and missing evidence never imply PASS.

Uses a bounded, instrumented build of cmd/simulator, not a synthetic write-rate
estimate. Run --help for exploratory versus capacity durations.
"""
import argparse
import concurrent.futures
import csv
import datetime as dt
import fcntl
import json
import math
import os
from pathlib import Path
import re
import shutil
import statistics
import subprocess
import threading
import time
import urllib.parse
import urllib.request

ROOT = Path(__file__).resolve().parents[1]
COMPOSE = ['docker', 'compose', '-f', str(ROOT/'loadtest/compose.benchmark.yaml')]
SENSORS = 'cpu,memory,disk_space,disk_busy,net_in,net_out,fan'.split(',')


def command(args, timeout=30):
    return subprocess.check_output(args, text=True, stderr=subprocess.STDOUT, timeout=timeout)


def sql(query):
    return command(COMPOSE+['exec', '-T', 'db', 'psql', '-U', 'monitor', '-d', 'monitor', '-At', '-c', "SET statement_timeout='15s'; "+query]).removeprefix('SET\n').strip()


def request(url, data=None):
    req = urllib.request.Request(url, data=json.dumps(data).encode() if data is not None else None,
                                 headers={'Content-Type':'application/json'})
    with urllib.request.urlopen(req, timeout=15) as response:
        return response.status, json.load(response) if data is None else response.read()


def queue():
    data = json.loads(command(COMPOSE+['exec','-T','nats','wget','-T','5','-qO-',
                                      'http://127.0.0.1:8222/jsz?consumers=true']))
    streams = [s for a in data['account_details'] for s in a.get('stream_detail',[]) if s['name']=='SERVER_METRICS']
    consumers = [c for s in streams for c in s.get('consumer_detail',[]) if c['name']=='db-writers']
    if len(consumers)!=1:
        raise RuntimeError('Missing/ambiguous durable consumer')
    c=consumers[0]
    return {'pending':c['num_pending'], 'ack_pending':c['num_ack_pending'], 'queue_bytes':streams[0]['state']['bytes']}


def drain(timeout=180):
    start=time.monotonic()
    while time.monotonic()-start < timeout:
        q=queue()
        if q['pending']+q['ack_pending']==0:
            return time.monotonic()-start
        time.sleep(2)
    raise RuntimeError('Queue did not drain within budget')


def percentile(values, q):
    if not values:
        return None
    values=sorted(values)
    return values[max(0, math.ceil(q*len(values))-1)]


def host_counters():
    cpu=list(map(int,Path('/proc/stat').read_text().splitlines()[0].split()[1:9]))
    device=os.stat(ROOT).st_dev
    disk=next(line.split() for line in Path('/proc/diskstats').read_text().splitlines()
              if line.split()[:2]==[str(os.major(device)),str(os.minor(device))])
    available=int(next(line.split()[1] for line in Path('/proc/meminfo').read_text().splitlines() if line.startswith('MemAvailable:')))*1024
    return dict(time=time.monotonic(), cpu_total=sum(cpu), cpu_idle=cpu[3]+cpu[4], disk_busy=int(disk[12]),
                disk_operations=int(disk[3])+int(disk[7]), disk_io_ms=int(disk[6])+int(disk[10]), available_memory=available)


def inventory(out):
    out.mkdir(parents=True,exist_ok=True)
    commands={'lscpu':['lscpu'], 'memory':['free','-h'], 'disks':['lsblk','-o','NAME,TYPE,ROTA,SIZE,MODEL'],
              'filesystems':['df','-h'], 'kernel':['uname','-a'], 'os':['cat','/etc/os-release'],
              'docker-version':['docker','version'], 'compose-version':['docker','compose','version'],
              'git-commit':['git','rev-parse','HEAD'], 'git-status':['git','status','--short'],
              'containers':['docker','ps','--no-trunc'], 'compose-ps':COMPOSE+['ps','-a']}
    for name,args in commands.items():
        (out/(name+'.txt')).write_text(command(args))
    ids=command(COMPOSE+['ps','-q']).split()
    inspect=json.loads(command(['docker','inspect']+ids))
    for item in inspect:
        item['Config'].pop('Env',None)
    (out/'container-inspect-redacted.json').write_text(json.dumps(inspect,indent=2))


def seed_history(base, out):
    # Real persisted test samples at hourly cadence, not a full 1 Hz year.
    end=dt.datetime.now(dt.timezone.utc).replace(minute=0,second=0,microsecond=0)
    samples=[]
    for hour in range(8761):
        when=end-dt.timedelta(hours=hour)
        samples.append(dict(device_id='benchmark-history', time=when.isoformat(),
                            **{s:round(45+20*math.sin(hour/24+i),2) for i,s in enumerate(SENSORS)}))
    for i in range(0,len(samples),400):
        status,_=request(base+'/api/v1/metrics/batch', {'samples':samples[i:i+400]})
        if status!=202:
            raise RuntimeError('History publication failed')
    drain()
    verify_history(out)


def verify_history(out):
    evidence=sql("SELECT count(*),count(DISTINCT time),min(time),max(time),extract(epoch FROM max(time)-min(time))/86400 FROM hour_rollup WHERE device_id='benchmark-history'")
    (out/'history-coverage.txt').write_text(evidence+'\nCadence: one actual sample/hour; all seven sensors; ingested through API and worker. Not a full 1 Hz annual dataset.\n')
    fields=evidence.split('|')
    if int(fields[0])!=8761 or float(fields[-1])<365:
        raise RuntimeError('History coverage verification failed')


def query_worker(base, stop, rows, lock, device):
    windows=[('1h',3600),('7d',7*86400),('365d',365*86400)]
    n=0
    while not stop.is_set():
        label, seconds=windows[n%3]; n+=1
        now=dt.datetime.now(dt.timezone.utc)
        query_device=device if label=='1h' else 'benchmark-history'
        params=urllib.parse.urlencode(dict(device_id=query_device, metrics=','.join(SENSORS),
            **{'from':(now-dt.timedelta(seconds=seconds)).isoformat(), 'to':now.isoformat(), 'max_points':1500}))
        started=time.time(); tick=time.perf_counter(); error=None
        try:
            status,data=request(base+'/api/v1/metrics/query?'+params)
            if status!=200 or not data['points'] or len(data['points'])>1500 or not any(p['metrics'] for p in data['points']):
                raise RuntimeError('Invalid or empty chart response')
        except Exception as exc:
            error=str(exc)
        with lock:
            rows.append(dict(time=started, window=label, ms=(time.perf_counter()-tick)*1000, error=error))
        stop.wait(1)


def generator_counters(path):
    rows=[line.split('BENCH ',1)[1] for line in path.read_text().splitlines() if 'BENCH ' in line]
    counters=dict(generated=0,accepted=0,failed=0,retries=0,missed_ticks=0,tick_lag_seconds=0,observed_at=time.time())
    if rows:counters.update(json.loads(rows[-1]))
    return counters


def step(args, out, devices, seconds, label):
    directory=out/label; directory.mkdir()
    drained=drain()
    prefix=re.sub('[^a-zA-Z0-9-]', '-', out.name+'-'+label)[:100]+'-'
    counter_sql="SELECT count(*),coalesce(extract(epoch FROM max(time)),0),count(DISTINCT device_id),extract(epoch FROM clock_timestamp()) FROM samples WHERE device_id LIKE '"+prefix+"%'"
    env={**os.environ, 'API_URL':args.api, 'DEVICE_COUNT':str(devices), 'HISTORY_DAYS':'0',
         'DEVICE_PREFIX':prefix, 'MAX_PUBLISH_ATTEMPTS':'2'}
    stop=threading.Event(); queries=[]; lock=threading.Lock()
    threads=[threading.Thread(target=query_worker,args=(args.api,stop,queries,lock,prefix+'00000'),daemon=True) for _ in range(10)]
    started=time.time(); deadline=started+seconds
    logpath=directory/'simulator.log'
    log=logpath.open('w')
    producer=subprocess.Popen([args.simulator],env=env,stdout=log,stderr=log)
    browser_log=(directory/'browser.log').open('w')
    browser=subprocess.Popen(['node',str(ROOT/'loadtest/browser_benchmark.cjs')],env={**os.environ,
        'API_URL':args.api,'SECONDS':str(seconds),'OUTPUT':str(directory/'browser.jsonl'),
        'QUERY_DEVICE':'benchmark-history'},stdout=browser_log,stderr=browser_log)
    for thread in threads: thread.start()
    samples=[]; errors=[]; reason=None; seen=set(); visibility=[]; health=[];previous_host=host_counters()
    try:
        while time.time()<deadline:
            tick=time.monotonic()
            try:
                now_dt=dt.datetime.now(dt.timezone.utc)
                lag_params=urllib.parse.urlencode(dict(device_id=prefix+f'{devices-1:05d}', metrics='cpu',
                    **{'from':(now_dt-dt.timedelta(seconds=60)).isoformat(),
                       'to':(now_dt+dt.timedelta(seconds=1)).isoformat(), 'max_points':1500}))
                ids=command(COMPOSE+['ps','-q']).split()
                with concurrent.futures.ThreadPoolExecutor(max_workers=5) as pool:
                    qfuture=pool.submit(queue)
                    dbfuture=pool.submit(sql,counter_sql)
                    statsfuture=pool.submit(command,COMPOSE+['stats','--no-stream','--format','json'])
                    lagfuture=pool.submit(request,args.api+'/api/v1/metrics/query?'+lag_params)
                    healthfuture=pool.submit(command,['docker','inspect']+ids)
                    q=qfuture.result(); fields=dbfuture.result().split('|')
                    resources=[json.loads(s) for s in statsfuture.result().splitlines() if s.startswith('{')]
                    if len(resources)!=4:raise RuntimeError('Incomplete Docker resource metrics')
                    lag_status, lag_data=lagfuture.result()
                    states=json.loads(healthfuture.result())
                now=time.time()
                if lag_status!=200:raise RuntimeError('Visibility query failed')
                for point in lag_data['points']:
                    if not point['metrics']:continue
                    timestamp=dt.datetime.fromisoformat(point['time'].replace('Z','+00:00')).timestamp()
                    if timestamp not in seen:
                        seen.add(timestamp)
                        if timestamp>=started+args.warmup:
                            visibility.append(now-float(timestamp))
                health.append(dict(time=now, containers=[dict(name=c['Name'], state=c['State'],
                    restarts=c['RestartCount'], cpu_quota=c['HostConfig']['CpuQuota'],
                    nano_cpus=c['HostConfig']['NanoCpus'], memory_limit=c['HostConfig']['Memory']) for c in states]))
                if len(states)!=4 or any(not c['State']['Running'] or c['State']['OOMKilled'] or c['RestartCount'] for c in states):
                    reason='Container stopped, restarted, or OOM killed'
                counters=generator_counters(logpath)
                free=shutil.disk_usage(ROOT).free
                meminfo=Path('/proc/meminfo').read_text()
                host=host_counters();host_elapsed=host['time']-previous_host['time']
                cpu_delta=host['cpu_total']-previous_host['cpu_total']
                disk_ops=host['disk_operations']-previous_host['disk_operations']
                row=dict(time=now, elapsed=now-started, devices=devices, phase='warmup' if now-started<args.warmup else 'measurement',
                         committed=int(fields[0]), newest=float(fields[1]), device_coverage=int(fields[2]), db_observed_at=float(fields[3]),
                         disk_free_bytes=free, host_cpu_percent=100*(1-(host['cpu_idle']-previous_host['cpu_idle'])/max(1,cpu_delta)),
                         disk_util_percent=100*(host['disk_busy']-previous_host['disk_busy'])/1000/host_elapsed,
                         disk_iops=disk_ops/host_elapsed,
                         disk_ms_per_operation=(host['disk_io_ms']-previous_host['disk_io_ms'])/max(1,disk_ops),
                         available_memory_bytes=host['available_memory'], **q, **counters)
                previous_host=host
                samples.append(row)
                with (directory/'resources.jsonl').open('a') as f:
                    f.write(json.dumps(dict(time=now, containers=resources, diskstats=Path('/proc/diskstats').read_text(),
                        netdev=Path('/proc/net/dev').read_text(), meminfo=meminfo,
                        generator_stat=Path(f'/proc/{producer.pid}/stat').read_text(),
                        generator_status=Path(f'/proc/{producer.pid}/status').read_text()))+'\n')
                (directory/'health.json').write_text(json.dumps(health))
                with (directory/'samples.csv').open('w') as f:
                    writer=csv.DictWriter(f,fieldnames=samples[0].keys());writer.writeheader();writer.writerows(samples)
                if free<10*1024**3: reason='Disk free below 10 GiB'
                if q['queue_bytes']>768*1024**2: reason='JetStream storage exceeded 768 MiB budget'
                if q['pending']+q['ack_pending']>500: reason='Queue backlog exceeded 500 messages'
                if counters['tick_lag_seconds']>10: reason='Generator tick lag exceeded 10 seconds'
                if producer.poll() is not None: reason='Generator exited unexpectedly'
                if browser.poll() is not None and time.time()<deadline-5:
                    errors.append('Browser workload exited before the declared duration')
                    reason='Essential browser metric collection failed'
                if host['available_memory']<512*1024**2:reason='Host available memory below 512 MiB'
                if len(samples)>12 and all(s['host_cpu_percent']>95 for s in samples[-12:]):reason='Host CPU above 95% for one minute'
                if len(samples)>12 and all(s['disk_util_percent']>95 for s in samples[-12:]):reason='Disk busy above 95% for one minute'
                if len(samples)>12 and all(s['pending']+s['ack_pending']>100 for s in samples[-12:]): reason='Persistent queue backlog above 100 messages for one minute'
                if reason: break
            except Exception as exc:
                errors.append(str(exc));reason='Essential metric collection failed';break
            time.sleep(max(0,args.sample_seconds-(time.monotonic()-tick)))
    finally:
        ended=time.time()
        producer.terminate()
        try: producer.wait(timeout=20)
        except subprocess.TimeoutExpired: producer.kill();producer.wait()
        stop.set()
        for thread in threads:thread.join(timeout=20)
        if ended<deadline and browser.poll() is None:browser.terminate()
        try: browser.wait(timeout=25)
        except subprocess.TimeoutExpired:browser.terminate();browser.wait(timeout=5)
        log.close();browser_log.close()
    (directory/'queries.json').write_text(json.dumps(queries))
    steady=[r for r in samples if r['phase']=='measurement']
    measured_queries=[q for q in queries if started+args.warmup<=q['time']<=ended]
    browser_rows=[]
    if (directory/'browser.jsonl').exists():
        browser_rows=[json.loads(r) for r in (directory/'browser.jsonl').read_text().splitlines()]
    browser_rows=[r for r in browser_rows if started+args.warmup<=r['time']<=ended]
    summary=dict(label=label, devices=devices, offered=devices, warmup=args.warmup, seconds=seconds,
                 drain_before_seconds=drained, stop_reason=reason, collection_errors=errors,
                 browser_exit=browser.returncode, generator_exit=producer.returncode, query={}, status='INCONCLUSIVE')
    for window in ['1h','7d','365d']:
        rows=[q for q in measured_queries if q['window']==window]
        summary['query'][window]={f'p{p}':percentile([q['ms'] for q in rows if not q['error']],p/100) for p in [50,95,99]}
        summary['query'][window].update(requests=len(rows),errors=sum(bool(q['error']) for q in rows))
    summary['browser_p95']=percentile([r['ms'] for r in browser_rows if not r.get('error') and r.get('ms') is not None],.95)
    summary['browser_errors']=sum(bool(r.get('error')) for r in browser_rows)
    summary['browser_requests']=len(browser_rows)
    summary['visibility_lag_p95_upper_bound']=percentile(visibility,.95)
    summary['visibility_probe_count']=len(visibility)
    failures=[]; missing=[]
    if len(steady)>=2:
        first,last=steady[0],steady[-1]; elapsed=last['time']-first['time']
        accepted_elapsed=last.get('observed_at',last['time'])-first.get('observed_at',first['time'])
        if accepted_elapsed<=0:accepted_elapsed=elapsed
        committed_elapsed=last['db_observed_at']-first['db_observed_at']
        xs=[r['time']-first['time'] for r in steady]
        ys=[r['pending']+r['ack_pending'] for r in steady]
        xmean=statistics.mean(xs);ymean=statistics.mean(ys)
        queue_slope=sum((x-xmean)*(y-ymean) for x,y in zip(xs,ys))/sum((x-xmean)**2 for x in xs)
        edge=max(1,min(12,len(steady)//3))
        queue_start=statistics.median(ys[:edge]);queue_end=statistics.median(ys[-edge:])
        summary.update(measured_seconds=elapsed, accepted_rate=(last['accepted']-first['accepted'])/accepted_elapsed,
            committed_rate=(last['committed']-first['committed'])/committed_elapsed,
            queue_slope=queue_slope, queue_start_median=queue_start, queue_end_median=queue_end,
            latest_sample_age_p95=percentile([r['time']-r['newest'] for r in steady if r['newest']],.95),
            generator_lag_p95=percentile([r['tick_lag_seconds'] for r in steady],.95))
        if summary['committed_rate']<.99*devices:failures.append('Committed rate below 99% offered')
        if summary['accepted_rate']<.99*devices:failures.append('Accepted rate below 99% offered')
        if queue_slope>0 and queue_end>queue_start+2:failures.append('Queue backlog grows beyond two-message transient allowance')
        if elapsed<900:missing.append('Less than 15 minutes measured')
    else: missing.append('Insufficient steady samples')
    if summary['browser_p95'] is None:missing.append('Browser p95 unavailable')
    elif summary['browser_p95']>=1000:failures.append('Browser p95 >= 1000ms')
    for window,q in summary['query'].items():
        if q['p95'] is None:missing.append(window+' query metrics unavailable')
        elif q['p95']>=1000:failures.append(window+' API p95 >= 1000ms')
        if q['requests'] and q['errors']/q['requests']>=.01:failures.append(window+' query error rate >=1%')
    if summary['browser_requests'] and summary['browser_errors']/summary['browser_requests']>=.01:failures.append('Browser error rate >=1%')
    summary['visibility_probe_method']='Last device in each generated tick, timestamp-to-chart-API visibility, polled every five seconds; conservative upper bound including polling delay.'
    if not visibility:missing.append('No end-to-end visibility lag probes')
    elif summary['visibility_lag_p95_upper_bound']>=10:missing.append('Visibility lag upper bound >=10 seconds; finer polling needed to determine actual violation')
    if not health:missing.append('No container health evidence')
    if any(c['cpu_quota']>0 or c['nano_cpus']>0 for h in health for c in h['containers']):
        missing.append('CPU quotas configured but cgroup throttle counters unavailable')
    if reason and not errors:failures.append(reason)
    summary.update(failures=failures, missing=missing)
    summary['status']='INCONCLUSIVE' if errors else 'FAIL' if failures else 'INCONCLUSIVE' if missing else 'PASS'
    try:
        summary['drain_after_seconds']=drain()
        final=sql(counter_sql).split('|'); summary['final_committed']=int(final[0]);summary['final_device_coverage']=int(final[2])
        counters=generator_counters(logpath);summary['final_accepted']=counters['accepted'];summary['final_failed']=counters['failed']
        summary['ingestion_error_rate']=counters['failed']/max(1,counters['generated'])
        if counters['failed']:
            summary['failures'].append('Unrecoverable publication failures / data loss')
            summary['status']='FAIL'
        if summary['final_committed']!=summary['final_accepted']:
            if producer.returncode==0:
                summary['failures'].append('Final committed / accepted counts differ after graceful shutdown and queue drain')
                summary['status']='FAIL'
            else:
                summary['missing'].append('Final committed / logged accepted counts differ; partial tick or loss requires reconciliation')
                if summary['status']=='PASS':summary['status']='INCONCLUSIVE'
        if summary['final_device_coverage']!=devices:
            summary['failures'].append('Incomplete device coverage')
            summary['status']='FAIL'
        # A process may terminate mid-tick; logged totals are a lower bound.
        summary['acceptance_counter_limitation']='Counters are emitted after each completed tick; termination can leave an unlogged partial tick.'
    except Exception as exc:summary['collection_errors'].append(str(exc));summary['status']='INCONCLUSIVE'
    (directory/'summary.json').write_text(json.dumps(summary,indent=2))
    print(json.dumps(summary),flush=True)
    return summary


def write_report(out, summaries):
    def number(value):
        return '—' if value is None else f'{value:.2f}'
    lines=[f'# Measured load-test results: {out.name}', '',
        'A maximum sustainable capacity requires repeated qualified steps and a soak run. This file reports individual observations only.', '',
        '[Declared workload and stop budgets](contract.json). [Verified historical coverage](history-coverage.txt).', '',
        '| Devices | Measured s | Accepted/s | Committed/s | Queue slope msg/s | Year API p95 ms | Browser p95 ms | Outcome |',
        '|---:|---:|---:|---:|---:|---:|---:|---|']
    for s in summaries:
        lines.append('| '+' | '.join([str(s['devices']),number(s.get('measured_seconds')),number(s.get('accepted_rate')),
            number(s.get('committed_rate')),number(s.get('queue_slope')),number(s['query']['365d']['p95']),
            number(s.get('browser_p95')),s['status']])+' |')
    for s in summaries:
        lines+=['',f"## {s['label']}: {s['status']}",'',f"[Full summary]({s['label']}/summary.json)."]
        for key in ['failures','missing','collection_errors']:
            if s.get(key):lines+=['',key.replace('_',' ').capitalize()+': '+ '; '.join(s[key])+'.']
        if s.get('final_committed') is not None:
            lines+=['',f"After drain: {s['final_committed']} stored samples; {s['final_accepted']} logged accepted samples; {s['final_device_coverage']} devices."]
    (out/'REPORT.md').write_text('\n'.join(lines)+'\n')


def main():
    global COMPOSE
    p=argparse.ArgumentParser(description=__doc__)
    p.add_argument('--devices',default='50,100,250,500,1000,2000,3000,5000,7500,10000')
    p.add_argument('--seconds',type=int,default=0,help='Override plan durations; exploratory only')
    p.add_argument('--warmup',type=int,default=120)
    p.add_argument('--sample-seconds',type=int,default=5)
    p.add_argument('--api',default='http://127.0.0.1:28081')
    p.add_argument('--project',default='server-monitoring-benchmark')
    p.add_argument('--simulator',default='/tmp/server-monitoring-simulator')
    p.add_argument('--out',type=Path,default=ROOT/'loadtest/results'/dt.datetime.now(dt.timezone.utc).strftime('run-%Y%m%dT%H%M%SZ'))
    p.add_argument('--skip-history',action='store_true')
    args=p.parse_args()
    COMPOSE=['docker','compose','-f',str(ROOT/'loadtest/compose.benchmark.yaml'),'--project-name',args.project]
    out=args.out.resolve();out.mkdir(parents=True,exist_ok=False)
    inventory(out/'before')
    contract={**vars(args), 'resource_thresholds':dict(disk_free_min_gib=10, queue_storage_max_mib=768,
        queue_max_messages=500, queue_persistent_messages=100, persistent_seconds=60, available_memory_min_mib=512,
        host_cpu_max_percent=95, disk_busy_max_percent=95, generator_lag_max_seconds=10),
        'queue_transient_allowance_messages':2,
        'visibility_probe':'One device through chart API every five seconds; upper-bound p95 < 10 seconds'}
    (out/'contract.json').write_text(json.dumps(contract,default=str,indent=2))
    if args.skip_history:verify_history(out)
    else:seed_history(args.api,out)
    summaries=[]
    try:
        for i,devices in enumerate(map(int,args.devices.split(','))):
            seconds=args.seconds or (300 if devices<1000 else 1030)
            result=step(args,out,devices,seconds,f'{i:02d}-{devices}')
            summaries.append(result)
            (out/'summary.json').write_text(json.dumps(summaries,indent=2))
            write_report(out,summaries)
            if result['stop_reason']:
                break
            if devices>=1000 and result['status']=='FAIL':
                break
    finally:
        inventory(out/'after')
    print('Results:',out,flush=True)
    return 0 if summaries and all(r['status']=='PASS' for r in summaries) else 2


if __name__=='__main__':
    with open('/tmp/server-monitoring-loadtest.lock','w') as lockfile:
        try:
            fcntl.flock(lockfile,fcntl.LOCK_EX|fcntl.LOCK_NB)
        except BlockingIOError:
            raise SystemExit('Another benchmark runner is active')
        raise SystemExit(main())
