import http from 'k6/http';
import {check} from 'k6';
export const options={vus:Number(__ENV.VUS||10),duration:__ENV.DURATION||'1m',thresholds:{http_req_duration:['p(95)<1000'],http_req_failed:['rate<0.01']}};
const base=__ENV.API_URL||'http://api:8080';
export default function(){let to=new Date().toISOString(),from=new Date(Date.now()-365*86400000).toISOString();let p=`device_id=${encodeURIComponent(__ENV.QUERY_DEVICE||'server-00000')}&metrics=cpu,memory,disk_space,disk_busy,net_in,net_out,fan&from=${encodeURIComponent(from)}&to=${encodeURIComponent(to)}&max_points=1500`;let r=http.get(base+'/api/v1/metrics/query?'+p);check(r,{'year query':r=>r.status===200&&JSON.parse(r.body).points.length<=1500})}
