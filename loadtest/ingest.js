import http from 'k6/http';
import { check, sleep } from 'k6';
export const options={vus:Number(__ENV.VUS||10),duration:__ENV.DURATION||'1m',thresholds:{http_req_failed:['rate<0.01']}};
const base=__ENV.API_URL||'http://api:8080';
export default function(){let sample={device_id:'load-'+__VU,time:new Date().toISOString(),cpu:25,memory:50,disk_space:55,disk_busy:16,net_in:18,net_out:29,fan:42};let r=http.post(base+'/api/v1/metrics/batch',JSON.stringify({samples:[sample]}),{headers:{'Content-Type':'application/json'}});check(r,{'published 202':r=>r.status===202});sleep(1)}
