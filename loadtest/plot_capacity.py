#!/usr/bin/env python3
"""Plot recorded steady-state samples only. Missing matplotlib is a hard error."""
import argparse
import csv
import json
from pathlib import Path
import matplotlib
matplotlib.use('Agg')
import matplotlib.pyplot as plt


def main():
    p=argparse.ArgumentParser();p.add_argument('--input',type=Path,required=True);args=p.parse_args()
    root=args.input
    summaries=[json.loads(path.read_text()) for path in sorted(root.glob('*/summary.json'))]
    if not summaries:raise RuntimeError('No measured summaries')
    run=root.name
    x=[s['devices'] for s in summaries]
    fig,ax=plt.subplots(figsize=(9,5))
    for key,label in [('offered','Offered'),('accepted_rate','HTTP 202 / NATS ack'),('committed_rate','DB committed')]:
        ax.plot(x,[s.get(key,float('nan')) for s in summaries],'-o',label=label)
    ax.set(xlabel='Devices',ylabel='Samples / second',title=f'{run}: steady-state throughput');ax.legend();ax.grid(alpha=.3)
    fig.tight_layout();fig.savefig(root/'01_write_rate.png',dpi=150);plt.close(fig)
    fig,ax=plt.subplots(figsize=(9,5))
    for window in ['1h','7d','365d']:
        for quantile,style in [('p50',':'),('p95','-'),('p99','--')]:
            ax.plot(x,[s['query'][window][quantile] or float('nan') for s in summaries],style+'o',label=f'API {window} {quantile}')
    ax.plot(x,[s.get('browser_p95') or float('nan') for s in summaries],'-s',label='Browser 365d p95')
    ax.axhline(1000,color='red',linestyle=':',label='1000 ms target')
    ax.set(xlabel='Devices',ylabel='Milliseconds',title=f'{run}: post-warmup query / rendering latency');ax.legend(fontsize=7,ncol=2);ax.grid(alpha=.3)
    fig.tight_layout();fig.savefig(root/'02_query_latency.png',dpi=150);plt.close(fig)
    # Docker reports CPU per logical core; 100% = one fully used core.
    resources={}
    for summary in summaries:
        previous_generator=None
        samples=list(csv.DictReader((root/summary['label']/'samples.csv').open()))
        steady_start=None
        if samples:steady_start=float(samples[0]['time'])-float(samples[0]['elapsed'])+summary['warmup']
        for line in (root/summary['label']/'resources.jsonl').read_text().splitlines():
            row=json.loads(line)
            fields=row['generator_stat'].split()
            generator_ticks=int(fields[13])+int(fields[14])
            if previous_generator:
                import os
                cpu=100*(generator_ticks-previous_generator[1])/os.sysconf('SC_CLK_TCK')/(row['time']-previous_generator[0])
                rss=int(next(s for s in row['generator_status'].splitlines() if s.startswith('VmRSS:')).split()[1])
                total=int(next(s for s in row['meminfo'].splitlines() if s.startswith('MemTotal:')).split()[1])
                if steady_start is not None and row['time']>=steady_start:
                    resources.setdefault((summary['devices'],'Simulator (host)'),[]).append(dict(CPUPerc=str(cpu),MemPerc=str(100*rss/total)))
            previous_generator=(row['time'],generator_ticks)
            if steady_start is None or row['time']<steady_start:continue
            for container in row['containers']:
                service=container.get('Service') or container['Name'].rsplit('-',2)[-2]
                key=(summary['devices'],service)
                resources.setdefault(key,[]).append(container)
    for field,filename,ylabel in [('CPUPerc','03_resource_usage.png','CPU (%; 100% = one core)'),('MemPerc','04_memory_usage.png','Memory (% of container limit / host RAM)')]:
        fig,ax=plt.subplots(figsize=(9,5))
        for service in sorted({key[1] for key in resources}):
            values=[]
            for devices in x:
                rows=resources.get((devices,service),[])
                values.append(max((float(r[field].rstrip('%')) for r in rows),default=float('nan')))
            ax.plot(x,values,'-o',label=service+' peak')
        ax.set(xlabel='Devices',ylabel=ylabel,title=f'{run}: measured container resources (5-second interval)');ax.legend();ax.grid(alpha=.3)
        fig.tight_layout();fig.savefig(root/filename,dpi=150);plt.close(fig)
    fig,axes=plt.subplots(2,1,figsize=(10,7))
    combined=[]
    for summary in summaries:
        rows=list(csv.DictReader((root/summary['label']/'samples.csv').open()))
        combined.extend(dict(step=summary['label'],**r) for r in rows)
        steady=[r for r in rows if r['phase']=='measurement']
        axes[0].plot([float(r['elapsed']) for r in steady],[int(r['pending'])+int(r['ack_pending']) for r in steady],label=summary['label'])
        axes[1].plot([float(r['elapsed']) for r in steady],[int(r['disk_free_bytes'])/1024**3 for r in steady],label=summary['label'])
    for ax,y in zip(axes,['Queue pending + ack pending (messages)','Filesystem free (GiB)']):
        ax.set(xlabel='Seconds from step start',ylabel=y);ax.legend();ax.grid(alpha=.3)
    fig.suptitle(run+': measured safety metrics (5-second interval)');fig.tight_layout();fig.savefig(root/'05_queue_disk.png',dpi=150);plt.close(fig)
    chosen=next((s for s in summaries if s['devices']==10000),summaries[-1])
    rows=[r for r in combined if r['step']==chosen['label'] and r['phase']=='measurement']
    if len(rows)>=2:
        fig,axes=plt.subplots(2,1,figsize=(10,7))
        times=[float(r['elapsed']) for r in rows[1:]]
        for counter,label in [('accepted','HTTP 202 / NATS ack'),('committed','DB committed')]:
            rates=[(int(b[counter])-int(a[counter]))/(float(b['time'])-float(a['time'])) for a,b in zip(rows,rows[1:])]
            axes[0].plot(times,rates,label=label)
        axes[0].axhline(chosen['devices'],linestyle=':',label='Offered')
        axes[1].plot([float(r['elapsed']) for r in rows],[int(r['pending'])+int(r['ack_pending']) for r in rows],label='Queue pending + ack pending')
        for ax,y in zip(axes,['Samples / second','Messages']):
            ax.set(xlabel='Seconds from step start',ylabel=y);ax.legend();ax.grid(alpha=.3)
        fig.suptitle(f"{run}: {chosen['devices']} devices, measured 5-second intervals");fig.tight_layout();fig.savefig(root/'06_throughput_time_series.png',dpi=150);plt.close(fig)
    with (root/'samples.csv').open('w') as f:
        writer=csv.DictWriter(f,fieldnames=combined[0].keys());writer.writeheader();writer.writerows(combined)
    if (root/'disk-intervals.json').exists():
        intervals=json.loads((root/'disk-intervals.json').read_text())
        fig,axes=plt.subplots(2,1,figsize=(10,7))
        for summary in summaries:
            rows=intervals.get(summary['label'],[])
            if not rows:continue
            samples=[r for r in combined if r['step']==summary['label']]
            start=float(samples[0]['time'])-float(samples[0]['elapsed'])
            rows=[r for r in rows if r['time']>=start+summary['warmup']]
            for ax,key in zip(axes,['iops','ms_per_operation']):
                ax.plot([r['time']-start for r in rows],[r[key] for r in rows],label=summary['label'])
        for ax,y in zip(axes,['Completed disk I/O operations / second','Average I/O time (ms / operation)']):
            ax.set(xlabel='Seconds from step start',ylabel=y);ax.legend();ax.grid(alpha=.3)
        fig.suptitle(run+': host filesystem device, /proc/diskstats interval deltas');fig.tight_layout();fig.savefig(root/'07_disk_io.png',dpi=150);plt.close(fig)


if __name__=='__main__':main()
