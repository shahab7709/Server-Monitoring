#!/usr/bin/env python3
"""Execute isolated capacity steps, then bounded 10k stress; preserve all volumes."""
import argparse
import json
import os
from pathlib import Path
import re
import shutil
import signal
import subprocess
import sys
import time
import urllib.request

ROOT=Path(__file__).resolve().parents[1]
COMPOSE=['docker','compose','-f',str(ROOT/'loadtest/compose.benchmark.yaml')]


def invoke(args, log=None):
    print('Running:', ' '.join(map(str,args)),flush=True)
    return subprocess.run(list(map(str,args)),cwd=ROOT,stdout=log,stderr=subprocess.STDOUT if log else None).returncode


def start(project, out):
    if invoke(COMPOSE+['--project-name',project,'up','-d']):raise RuntimeError('Stack startup failed')
    for _ in range(90):
        try:
            with urllib.request.urlopen('http://127.0.0.1:28081/health/ready',timeout=3) as response:
                if response.status==200:break
        except Exception:pass
        time.sleep(1)
    else:raise RuntimeError('API readiness failed')
    with (out/'smoke.log').open('w') as log:
        if invoke(['docker','run','--rm','--network',project+'_default','-e','API_URL=http://api:8080','server-monitoring-smoke'],log):
            raise RuntimeError('Smoke test failed; see smoke.log')


def main():
    p=argparse.ArgumentParser(description=__doc__)
    p.add_argument('--devices',default='50,100,250,500,1000,2000,3000,5000,7500,10000')
    p.add_argument('--out',type=Path,required=True)
    p.add_argument('--simulator',default='/tmp/server-monitoring-simulator-next')
    p.add_argument('--plot-python',default='/tmp/server-monitoring-loadtest-venv/bin/python')
    args=p.parse_args();out=args.out.resolve();out.mkdir(parents=True,exist_ok=False)
    base='sm-benchmark-'+re.sub('[^a-z0-9-]','-',out.name.lower())[:45]
    # Only a deployment created for this benchmark is stopped. Production is untouched.
    if invoke(COMPOSE+['stop']):raise RuntimeError('Could not stop previous benchmark stack')
    results={}
    for phase,devices,extra in [('capacity',args.devices,[]),('stress','10000',['--seconds','180','--warmup','20'])]:
        if phase=='stress' and shutil.disk_usage(ROOT).free<12*1024**3:
            results[phase]={'status':'NOT RUN','reason':'Less than 12 GiB free; insufficient headroom for a fresh stress stack'}
            break
        project=base+'-'+phase
        try:
            start(project,out)
            runner=[sys.executable,str(ROOT/'loadtest/capacity_benchmark.py'),'--devices',devices,
                '--project',project,'--simulator',args.simulator,'--out',str(out/phase)]+extra
            with (out/(phase+'.log')).open('w') as log:
                results[phase]={'exit_code':invoke(runner,log),'project':project}
            if (out/phase/'summary.json').exists():
                results[phase]['steps']=json.loads((out/phase/'summary.json').read_text())
                with (out/(phase+'-plots.log')).open('w') as log:
                    results[phase]['plot_exit_code']=subprocess.run([args.plot_python,str(ROOT/'loadtest/plot_capacity.py'),'--input',str(out/phase)],
                        env={**os.environ,'MPLCONFIGDIR':'/tmp/server-monitoring-matplotlib'},stdout=log,stderr=subprocess.STDOUT).returncode
            else:
                results[phase]['status']='INCONCLUSIVE: runner did not produce summary'
                break
            if any(s['collection_errors'] for s in results[phase]['steps']):
                results[phase]['status']='INCONCLUSIVE: repair instrumentation before continuing'
                break
        finally:
            invoke(COMPOSE+['--project-name',project,'stop'])
            (out/'execution.json').write_text(json.dumps(results,indent=2))
    (out/'execution.json').write_text(json.dumps(results,indent=2))
    print('Execution evidence:',out,flush=True)
    return 0 if results and all(r.get('exit_code')==0 and r.get('plot_exit_code')==0 for r in results.values()) else 2


if __name__=='__main__':
    if sys.argv[1:]==['--stop']:
        # Stop only this task's active benchmark processes; never start a workload.
        for script in ['execute_plan.py','capacity_benchmark.py']:
            for process in Path('/proc').iterdir():
                if not process.name.isdigit() or int(process.name)==os.getpid():continue
                try:argv=(process/'cmdline').read_bytes().decode().split('\0')
                except (OSError,UnicodeDecodeError):continue
                if any(a.endswith('/'+script) or a=='loadtest/'+script for a in argv) and any('completion-20261104' in a for a in argv):
                    try:os.kill(int(process.name),signal.SIGINT);print('Stopping',script,process.name,flush=True)
                    except ProcessLookupError:pass
        raise SystemExit(0)
    raise SystemExit(main())
