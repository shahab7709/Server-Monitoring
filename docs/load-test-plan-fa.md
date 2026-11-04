# برنامه اجرای تست بار و تعیین ظرفیت — Server Monitoring

**وضعیت:** آماده اجرا روی ماشین شما؛ هیچ نتیجه‌ای در این سند اندازه‌گیری نشده است.  
**محدوده:** API، NATS JetStream، Worker، TimescaleDB، Simulator، API نمودار و مصرف منابع Docker  
**مبنای طراحی:** `docs/feature-spec-v2.md` (F07، F08، F12؛ TC28، TC46–TC49)، `compose.yaml` و اسکریپت‌های موجود `loadtest/`.

## ۱. هدف و پرسش‌های قابل پاسخ

1. بیشترین تعداد **دستگاه پایدار** روی سخت‌افزار آزمایش‌شده چقدر است، در حالی که هر دستگاه هر ثانیه یک Sample با ۷ سنسور می‌فرستد؟
2. نرخ **پذیرش HTTP** و **ثبت پایدار در دیتابیس** چقدر است و آیا backlog صف با گذشت زمان افزایش می‌یابد؟
3. تأخیر پاسخ نمودار در بار هم‌زمان و با تاریخچه واقعی چقدر است (p50/p95/p99)؟
4. محدودکنندهٔ ظرفیت کدام جزء است (تولیدکننده، API، NATS، Worker، DB، دیسک یا Query) و شواهد آن چیست؟
5. در سناریوی ۱۰٬۰۰۰ دستگاه، نخستین نقطهٔ شکست کجاست و چه تغییری با چه ترتیب اولویتی پیشنهاد می‌شود؟

**تعریف ظرفیت:** بزرگ‌ترین پله دستگاه‌ها که حداقل ۱۵ دقیقه در حالت پایدار **تمام شرایط پذیرش** بخش ۵ را پاس کند. «HTTP 202» صرفاً به معنی تأیید انتشار پایدار در صف است، نه commit در دیتابیس. از نتیجهٔ یک اجرای کوتاه به‌عنوان ظرفیت پایدار استفاده نکنید.

## ۲. شرایط و مشخصات سخت‌افزار

آزمایش را روی همان ماشینی انجام دهید که Docker Compose روی آن در حال اجراست. ثبت موارد زیر **الزامی** است و در گزارش نهایی باید همراه با تاریخ و شناسه commit درج شود:

| فیلد | دستور/روش ثبت | مقدار واقعی |
|---|---|---|
| CPU مدل / هسته منطقی | `lscpu` | تکمیل پس از اجرا |
| RAM کل/آزاد | `free -h` | تکمیل پس از اجرا |
| دیسک، نوع SSD/HDD و ظرفیت | `lsblk -o NAME,TYPE,ROTA,SIZE,MODEL` ، `df -h` | تکمیل پس از اجرا |
| سیستم‌عامل/Kernel | `uname -a` ، `cat /etc/os-release` | تکمیل پس از اجرا |
| نسخه Docker/Compose | `docker version` ، `docker compose version` | تکمیل پس از اجرا |
| محدودیت کانتینرها | `docker inspect` یا فایل Compose نهایی | تکمیل پس از اجرا |
| مشخصات DB/NATS | نسخه image و اندازه volume | تکمیل پس از اجرا |
| پیکربندی داده | retention، شمار سنسورها، interval، تاریخچه، اندازه batch | تکمیل پس از اجرا |
| نسخه کد | `git rev-parse HEAD`، `git status --short` | تکمیل پس از اجرا |

نمونه جمع‌آوری اولیه (در Linux):

```bash
mkdir -p loadtest/results/run-$(date -u +%Y%m%dT%H%M%SZ)
# RUN_DIR را روی مسیر ایجادشده تنظیم کنید.
RUN_DIR="$(ls -dt loadtest/results/run-* | head -1)"
lscpu > "$RUN_DIR/lscpu.txt"
free -h > "$RUN_DIR/memory.txt"
lsblk -o NAME,TYPE,ROTA,SIZE,MODEL > "$RUN_DIR/disks.txt"
df -h > "$RUN_DIR/filesystems.txt"
uname -a > "$RUN_DIR/kernel.txt"
docker version > "$RUN_DIR/docker-version.txt"
docker compose version > "$RUN_DIR/compose-version.txt"
docker compose config > "$RUN_DIR/compose-effective.yaml" # پیش از اشتراک‌گذاری، اسرار را حذف کنید.
docker compose ps > "$RUN_DIR/compose-ps.txt"
git rev-parse HEAD > "$RUN_DIR/git-commit.txt"
```

**نکته:** `compose-effective.yaml` ممکن است credentials داشته باشد؛ نسخهٔ قابل انتشار را sanitize کنید. فضای DB و filesystem و محدودیت‌های cgroup را نیز قبل و بعد هر مرحله ثبت کنید. اگر سیستم‌عامل macOS/Windows است، CPU/RAM تخصیص‌داده‌شده به Docker Desktop را جدا از مشخصات Host گزارش کنید.

## ۳. آماده‌سازی و ایزوله‌کردن بار

- از دیتابیس و volumeهای موجود backup بگیرید؛ تست ممکن است دادهٔ زیادی تولید کند. **هیچ `docker compose down -v` اجرا نکنید** مگر در محیط اختصاصی و با اجازه.
- مطمئن شوید فقط **یک** Simulator بارساز فعال است. سرویس `simulator` پیش‌فرض Compose را قبل از پله‌ها متوقف کنید تا با `docker compose run` هم‌پوشانی نداشته باشد:

  ```bash
  docker compose ps
  docker compose stop simulator
  docker compose up -d db nats api worker
  curl -fsS http://localhost:8080/ >/dev/null
  ```

  `curl /` فقط بررسی اولیه است؛ موفقیت API، DB و Queue را با smoke test پروژه نیز تأیید کنید. قبل از توقف یک Simulator موجود، بررسی کنید توقف آن برای سرویس زنده مجاز است.
- Schedulerهای backfill/retention و jobهای سنگین را ثبت کنید؛ در تست ظرفیت بار اضافیِ کنترل‌نشده نداشته باشید.
- از مجموعه دستگاه/کلید داده مخصوص هر run استفاده کنید یا آزمون‌ها را روی دیتابیس مجزا انجام دهید. در غیر این صورت dedup با `(device_id,time)`، دادهٔ باقی‌مانده و catch-up صف می‌تواند نرخ نوشته‌شدن را منحرف کند.
- برای تست Query یک‌ساله، دیتاست ۳۶۵ روزهٔ **واقعی و مشخص** با Rollup ساعتی بسازید (ممکن است با backfill مستقیم/seed به DB جداگانه انجام شود). تنظیم `HISTORY_DAYS=0` برای این تست کافی نیست. بعد از seed، تعداد دستگاه/باکت و پوشش زمانی داده را assert کنید.
- منبع بار را از نظر CPU/RAM/Network زیر نظر بگیرید. اگر شبیه‌ساز به سقف خود رسید، نتیجه ظرفیت **load generator** است، نه ظرفیت سامانه؛ بارساز را روی ماشین جدا یا با چند producer مقیاس دهید و دوباره اجرا کنید.

## ۴. ابزار موجود در مخزن و اصلاحات لازم قبل از ادعای ظرفیت

| ابزار/مسیر | کاربرد | نکته/اقدام ضروری |
|---|---|---|
| `cmd/simulator/main.go` | تولید ۱ Sample در ثانیه برای هر Device؛ Batchهای حداکثر ۴۰۰تایی | به‌علت ارسال ترتیبی batch و retry نامحدود، نرخ بار واقعی را از نرخ هدف جداگانه بسنجید. در ۱۰k دستگاه حدود ۲۵ batch در هر tick نیاز است. |
| `loadtest/capacity_benchmark.py` | پله‌های Device، نرخ DB، latency Query، CPU/RAM | `queue_pending()` فعلاً `None` می‌دهد؛ بدون backlog نمی‌توان پایداری مصرف صف را اثبات کرد. |
| `loadtest/plot_capacity.py` | رسم چهار PNG براساس CSV واقعی | نمودار را **تنها** از دادهٔ اندازه‌گیری‌شده بسازید؛ پله‌های Warmup و داده‌های نامعتبر را از خلاصه حذف کنید. |
| `loadtest/ingest.js` | k6 بار HTTP | مکمل شبیه‌ساز است، نه جایگزین سناریوی «تعداد دستگاه». |
| `loadtest/query.js` | k6 بار Query یک‌ساله | برای ۱۰ VU نیاز به تاریخچه واقعی ۳۶۵روزه و اندازه‌گیری مرورگر جداگانه است. |

### اصلاحات ضروری در runner (بهتر است به صورت PR کوچک انجام شوند)

1. `queue_pending()` را با NATS monitoring API / JetStream consumer info پیاده کنید؛ `num_pending` و `num_ack_pending` و نرخ رشدشان را هر ۵ ثانیه ثبت کنید. در Compose پورت ۸۲۲۲ درون شبکه موجود است، اما روی Host منتشر نشده؛ با دسترسی امن درون شبکه نمونه‌گیری کنید.
2. نرخ ثبت موفق DB را از شمارنده‌ای **قابل اعتبارسنجی** محاسبه کنید. `pg_stat_user_tables.n_tup_ins` یک شمارنده تجمعی PostgreSQL است، تحت تأثیر reset/statistics و شرایط درج، و به‌تنهایی اثبات نمی‌کند تمام Sampleها ثبت شده‌اند. برای هر run از شناسه‌ها/بازه‌های مجزا و `COUNT(*)` دقیق در ابتدا و انتها یا metric commit موفق و lag استفاده کنید؛ نرخ را برحسب **زمان دقیق** محاسبه کنید.
3. اندازه‌گیری p95/p99 را **برای هر پله و پس از Warmup** از درخواست‌های همان پله انجام دهید. در runner فعلی آرایه latency حین Warmup نیز جمع می‌شود و نمونه‌های CSV دارای p95 تجمعی هستند.
4. بعد از هر پله، Simulator قبلی را متوقف کنید و صف را **تا رسیدن به وضعیت پایه** تخلیه/پایدار کنید؛ در غیر این صورت نرخ نوشته‌شدنِ پله بعدی ممکن است متعلق به دستگاه‌های پله قبل باشد.
5. نمونه‌های زمانی را ثبت کنید، نه فقط میانگین هر پله. علاوه بر CPU/RAM، DB disk IOPS/latency، فضای دیسک، network، شمار queryهای خطادار، نرخ ورودی و backlog را نگه دارید.
6. خطای `docker stats` / `psql` / نبود `matplotlib` نباید به‌صورت موفقیت تلقی شود. اگر metric ضروری موجود نیست، نتیجهٔ آن پله `INCONCLUSIVE` باشد.
7. برای تست ۱۰k، retry نامحدود در شبیه‌ساز می‌تواند آن را برای مدت طولانی متوقف کند؛ موفق/ناموفق بودن publish و backlog سمت تولیدکننده را با timeout و شمارنده جداگانه ثبت کنید.
8. اسکریپت خلاصه، **PASS/FAIL/INCONCLUSIVE** هر پله را با آستانه‌ها و شواهد محاسبه و exit code غیرصفر روی عدم احراز شرط‌ها تولید کند.

## ۵. شاخص‌ها، آستانه‌ها و تعریف «بیشترین ظرفیت»

| شاخص | تعریف و روش اندازه‌گیری | معیار اولیه (قابل تنظیم و اعلام قبل از آزمون) |
|---|---|---|
| Offered load | `devices × 1 sample/s` | مرجع؛ به‌تنهایی نرخ واقعی نیست |
| Actual accepted rate | Sampleهای موجود در HTTP 202 / ثانیه، نه فقط request/s | نزدیک offered؛ بدون retry متراکم و بدون افزایش backlog تولیدکننده |
| DB committed rate | Sampleهای جدید commit‌شده / ثانیه | `>= 99%` نرخ offered در پنجره پایدار (پیشنهاد آزمایش، نه الزام spec) |
| End-to-end lag | p95 زمان تولید Sample تا قابل Query شدن آن | `< 10s` پیشنهاد اولیه؛ باید در گزارش به‌عنوان فرض مشخص شود |
| Queue backlog slope | تغییر پیام‌های Pending + Ack Pending در طول پنجره پایدار | `<= 0` با تحمل نوسان کوتاه‌مدت؛ برای هر پله واضح گزارش شود |
| Write / ingest errors | 4xx/5xx، publish failures، retry | `< 1%` خطای غیرقابل بازیابی؛ بدون از دست رفتن داده |
| Query API latency | p50/p95/p99 برای ۱h، ۷d و ۳۶۵d، با ۱۰ VU | p95 < ۱۰۰۰ms برای **End-to-End نمایش یک‌ساله**؛ API تنها شرط کافی نیست |
| Browser chart latency | از شروع درخواست تا نمایش کامل نمودار، p95 در مرورگر | `< 1000ms` با دیتاست یک‌ساله، ۷ سنسور، ۱۰ کاربر هم‌زمان (هدف پیشنهادی spec) |
| Resource headroom | CPU، RAM، IO، storage، network همه سرویس‌ها و Host | نبود OOM / throttling شدید / saturation مداوم؛ سقف دلخواه باید پیشاپیش ثبت شود |

**قاعده PASS:** یک پله فقط وقتی `PASS` است که حداقل ۱۵ دقیقه پس از ۲ دقیقه Warmup، نرخ ذخیره، backlog، error rate، تأخیر API/نمودار و سلامت منابع در محدوده باشند. اگر مرورگر یا Queue metric موجود نباشد، از عنوان «ظرفیت کامل با SLO» استفاده نکنید و وضعیت را `INCONCLUSIVE` بگذارید. پیشنهاد: هر پله PASS نزدیک سقف را ۳ مرتبه اجرا کنید و ظرفیت محافظه‌کارانه را گزارش کنید.

## ۶. سناریوها و ترتیب اجرا

### A — Baseline (بدون فشار)

- با `50` و `100` دستگاه، هر پله ۵ دقیقه شامل ۲ دقیقه Warmup، سلامت Compose، Query و correctness را تأیید کنید.
- اطمینان از پوشش دستگاه‌ها و نرخ دقیق تولید را بررسی کنید. نتایج این بخش به‌عنوان benchmark اولیه ثبت شوند، نه ظرفیت نهایی.

### B — Step Load / Capacity Search

- پله‌های پیشنهادی: **100 → 250 → 500 → 1000 → 2000 → 3000 → 5000 → 7500 → 10000**.
- در پله‌های اولیه ۵ دقیقه برای کشف مشکل، از ۱۰۰۰ به بعد هر پله **۱۷ دقیقه (۲ دقیقه Warmup + ۱۵ دقیقه اندازه‌گیری)**.
- هم‌زمان با ingestion، بار Query `10 VU` با توزیع ۱h/۷d/۳۶۵d وارد کنید. Query یک‌ساله فقط پس از تکمیل دیتاست تاریخی معتبر است.
- بعد از شکست، جستجوی دودویی بین آخرین پله PASS و اولین FAIL انجام دهید (مثلاً ۳۰۰۰ PASS و ۵۰۰۰ FAIL ⇒ ۴۰۰۰).
- پس از پایان هر پله، backlog را تخلیه/اندازه‌گیری و منابع را به وضعیت قابل مقایسه برگردانید. مدت زمان تخلیه جداگانه مهم است.

### C — Soak / Stability

- بیشترین پله PASS را **۳۰ تا ۶۰ دقیقه** ادامه دهید و نشتی RAM، رشد backlog و افت throughput را بسنجید.
- برای ظرفیت قطعی، معیار ۱۵ دقیقه‌ای را با soak و اجرای تکراری تأیید کنید.

### D — Stress تا ۱۰٬۰۰۰ دستگاه

- ۱۰k را حتی در صورت شکست اجرا کنید **فقط اگر محیط آزمایش اختصاصی است و بودجهٔ دیسک/صف و stop condition وجود دارد**.
- اگر queue backlog به‌صورت پایدار افزایش یافت، latency به‌شدت رشد کرد، DB به فضای بحرانی رسید، OOM رخ داد یا commit rate افت کرد، تست را متوقف کنید؛ نیازی به ایجاد اختلال تخریبی نیست.
- منابع را جداگانه بررسی کنید تا «اولین گلوگاه» بر اساس داده مشخص شود، نه حدس.

## ۷. دستورهای اجرا روی سیستم شما

ابتدا در ریشه پروژه:

```bash
docker compose ps
docker compose stop simulator
docker compose up -d db nats api worker
python3 --version
python3 -m pip install matplotlib   # فقط برای تولید PNG؛ در venv ترجیح دارد
```

اسکریپت **موجود** برای یک اجرای اکتشافی کوتاه (پیش از اصلاحات بخش ۴، نتایج صرفاً مقدماتی‌اند):

```bash
python3 loadtest/capacity_benchmark.py \
  --devices 50,100,250,500,1000 \
  --seconds 120 --warmup 20 --sample-seconds 5 \
  --query-workers 2 --query-window-days 0.0416667 \
  --out loadtest/results/exploratory

python3 loadtest/plot_capacity.py --input loadtest/results/exploratory
```

**مهم:** runner فعلی p95 را از کل پله شامل Warmup جمع‌آوری می‌کند، Queue Pending را نمی‌خواند، و `--query-workers` فقط هم‌زمانی HTTP را تعیین می‌کند. تا پیاده‌سازی اصلاحات، این فرمان برای تعیین ظرفیت نهایی یا اثبات SLO یک‌ساله مناسب نیست.

نمونه اجرای مستقل k6 برای Query یک‌ساله (فقط **بعد از** آماده‌سازی داده یک‌ساله):

```bash
docker run --rm --network server-monitoring_default \
  -v "$PWD/loadtest:/scripts:ro" \
  -e API_URL=http://api:8080 -e VUS=10 -e DURATION=15m \
  -e QUERY_DEVICE=server-00000 \
  grafana/k6 run /scripts/query.js
```

نام شبکه معمولاً `server-monitoring_default` است؛ با `docker network ls` بررسی کنید. کد `query.js` فعلی فقط HTTP را می‌سنجد؛ اندازه‌گیری زمان نمایش در مرورگر باید جداگانه (مثلاً Playwright Performance API) اضافه شود. برای این سناریو مقدار query یک‌ساله و توزیع داده واقعی را لاگ کنید.

## ۸. نمودارهای الزامی و داده‌های آنها

خروجی باید شامل نمودارهای واقعی و دارای محور، واحد، بازه نمونه‌برداری و مشخصات run باشد:

1. **نرخ نوشتن (Write Throughput):** محور X تعداد Device، محور Y Sample/s. سه سری: offered، accepted by NATS، persisted in DB. نمودار زمان‌محور مخصوص پله ۱۰k نیز برای اثبات پایداری/رشد backlog اضافه شود.
2. **زمان پاسخ نمودار (Chart Response Time):** محور X تعداد Device، محور Y میلی‌ثانیه. p50، p95، p99 API و **p95 نمایش مرورگر** به‌صورت مجزا؛ نمودار ۱h، ۷d و ۳۶۵d را تفکیک کنید. هدف ۱۰۰۰ms را به‌صورت خط مرجع مشخص کنید.
3. **مصرف منابع (Resource Usage Under Load):** CPU و RAM به تفکیک API، Worker، TimescaleDB، NATS و Simulator؛ حداقل یک نمودار CPU و یک نمودار RAM. نمودار Disk IO و Queue backlog برای تشخیص گلوگاه توصیه می‌شود.

اسکریپت موجود `plot_capacity.py` از `samples.csv` چهار PNG به نام‌های `01_write_rate.png`، `02_query_latency.png`، `03_resource_usage.png` و `04_memory_usage.png` می‌سازد. اینها نقطهٔ شروع هستند، **اما** سری پذیرفته‌شده توسط NATS، backlog و p95 نمایش مرورگر را ندارند و باید به instrumentation افزوده شوند. هیچ داده نمونه یا تخمینی نباید با عنوان «measured» ارائه شود.

## ۹. روش شناسایی گلوگاه

| علامت مشاهده‌شده | تفسیر محتمل | اقدام تشخیصی / پیشنهاد اول |
|---|---|---|
| Offered بیشتر از Accepted، CPU Simulator بالا | محدودیت تولیدکننده یا HTTP / ارسال ترتیبی Batch | Producer جدا، parallel publish کنترل‌شده، ثبت مدت ارسال Batch |
| Accepted بالا، Persisted پایین، backlog صف پیوسته صعودی | مصرف‌کننده/DB از ingestion عقب افتاده | Consumer lag، زمان transaction، lock و DB IO را مقایسه کنید |
| Worker CPU بالا، DB کم‌بار | پردازش/serialization و مسیر تک‌Worker | profiling، batching و concurrency با حفظ ordering/idempotency |
| DB CPU یا IO و lock wait بالا | raw + دو rollup در transaction، index/write amplification | EXPLAIN/pg_stat_statements، batching، index و rollup pipeline را بررسی کنید |
| RAM صف یا disk volume بالا و backpressure | پیام‌های مصرف‌نشده یا محدودیت storage JetStream | اصلاح consumer throughput، ظرفیت و retention صف؛ نه فقط افزایش RAM |
| p95 نمودار بالا، ولی ingest پایدار | Query plan/DB contention یا رندر UI | EXPLAIN ANALYZE، query concurrency، browser tracing؛ تفکیک API از render |
| CPU میزبان اشباع / throttling | Host یا محدودیت cgroup | ثبت Host utilization و افزایش منابع یا تفکیک نقش‌ها |

**فرضیهٔ کدمحور:** در نسخه فعلی شبیه‌ساز ارسال Batchها را ترتیبی انجام می‌دهد و هر Sample در Worker همراه با raw و دو Rollup پردازش می‌شود؛ اینها *نامزد گلوگاه* هستند. گلوگاه واقعی فقط با همبستگی نرخ‌ها، lag، profiler و resource metrics در دستگاه شما مشخص خواهد شد.

## ۱۰. پاسخ تحلیلی به سناریوی ۱۰٬۰۰۰ دستگاه

با فرض ۱ نمونه برای هر دستگاه در هر ثانیه:

- **۱۰٬۰۰۰ Sample/s** یا **۸۶۴ میلیون Sample در روز**؛ هر Sample دارای ۷ سنسور است.
- با Batch حداکثر **۴۰۰ Sample**، حداقل **۲۵ Batch در ثانیه** لازم است؛ overhead شبکه، JSON، index، WAL و دو Rollup هنوز محاسبه نشده‌اند.
- در این حجم، نگه‌داری ۷ روز raw بدون محاسبه ظرفیت دیسک/WAL و فشرده‌سازی ریسک بزرگی است؛ حجم بایتی باید با داده واقعی اندازه‌گیری شود.

**ترتیب بررسی و اصلاح پیشنهادی (مشروط به شواهد):**

1. **ابتدا نرخ واقعی Simulator را تأیید کنید.** اگر حتی به ۱۰k Sample/s عرضه نمی‌کند، parallel producer یا چند generator مستقل ایجاد کنید. throughput ناموفقِ مولد را به DB نسبت ندهید.
2. **اگر backlog بالا می‌رود:** زمان commit و سرعت Worker/DB را پروفایل کنید؛ batchهای DB، statementهای bulk، تعداد consumerها و `MaxAckPending` را با تست A/B تنظیم کنید. بالا بردن concurrency بدون بررسی transaction conflicts و dedup خطرناک است.
3. **اگر DB اشباع است:** هزینه raw + rollup transactional را بسنجید؛ تنظیم index/Timescale chunk و compression، WAL/IO، batch insert و در صورت لزوم انتقال rollup به pipeline async با تضمین correctness/replay را ارزیابی کنید.
4. **اگر نمودار کند می‌شود:** queryها را روی rollupهای زمانی و حداکثر ۱۵۰۰ bucket نگه دارید، query plan و cache را بسنجید، منابع Query و ingest را در صورت نیاز جدا کنید.
5. **اگر IO یا فضای دیسک محدود است:** storage SSD سریع‌تر/بیشتر، retention و compression، monitoring فضای WAL و JetStream را طراحی کنید.

**جملهٔ قابل دفاع برای گزارش:** «در ۱۰٬۰۰۰ دستگاه انتظار ۱۰٬۰۰۰ Sample/s داریم؛ اولین جزء شکست‌خورده را تنها بعد از مشاهدهٔ نرخ واقعی تولید/پذیرش/commit، رشد صف، p95 Query و CPU/RAM/IO اعلام می‌کنیم.» تا اجرای تست واقعی، نام یک جزء مشخص را به‌عنوان «اولین شکست قطعی» گزارش نکنید.

## ۱۱. قالب گزارش نهایی پس از اجرا

### مشخصات اجرا

- تاریخ UTC: `_____`؛ Commit: `_____`؛ Docker/Compose: `_____`
- CPU: `_____`؛ RAM: `_____`؛ Disk: `_____`؛ OS: `_____`
- داده تاریخی Query: `_____` روز؛ Device: `_____`؛ VU: `_____`
- سیاست شرایط پذیرش: `_____`؛ مدت Warmup: `_____`؛ مدت پنجره اندازه‌گیری: `_____`

### جدول نتایج

| Devices | Offered samples/s | Accepted/s | DB committed/s | Queue slope | Query p95 ms | Browser p95 ms | Max CPU DB/Worker | Error % | PASS/FAIL |
|---:|---:|---:|---:|---:|---:|---:|---|---:|---|
| 100 | — | — | — | — | — | — | — | — | — |
| 500 | — | — | — | — | — | — | — | — | — |
| 1000 | — | — | — | — | — | — | — | — | — |
| 2000 | — | — | — | — | — | — | — | — | — |
| 5000 | — | — | — | — | — | — | — | — | — |
| 10000 | — | — | — | — | — | — | — | — | — |

### شواهد و جمع‌بندی

- بیشترین ظرفیت پایدار روی سخت‌افزار ثبت‌شده: `_____` دستگاه (با SLO تعریف‌شده).
- اولین پله FAIL: `_____` دستگاه؛ معیار شکست: `_____`.
- گلوگاه اثبات‌شده: `_____`؛ شواهد از نمودارها و لاگ‌ها: `_____`.
- اجرای مجدد/تأیید: `_____`.
- اقدام اصلاحی P0/P1 و پیش‌بینی اثر (به‌صورت فرضیه تا آزمون A/B): `_____`.
- تصاویر الزامی: `01_write_rate.png`، `02_query_latency.png`، `03_resource_usage.png` و `04_memory_usage.png`، همراه با `samples.csv` و فایل مشخصات سخت‌افزار.

## ۱۲. Definition of Done

- [ ] سخت‌افزار Host و محدودیت Docker ثبت شده است.
- [ ] هیچ producer اضافی، داده تکراری یا backlog منتقل‌شده بین پله‌ها وجود ندارد.
- [ ] نرخ offered، accepted و committed به‌صورت مستقل اندازه‌گیری شده است.
- [ ] Queue depth/lag و ظرفیت دیسک موجود است.
- [ ] آزمون ۱ ساله با دادهٔ واقعی و ۱۰ VU انجام شده است.
- [ ] p95 نمایش واقعی نمودار (نه فقط API) اندازه‌گیری شده است.
- [ ] همه پله‌ها با PASS/FAIL/INCONCLUSIVE و آستانه از پیش ثبت‌شده گزارش شده‌اند.
- [ ] حداقل سه نمودار خواسته‌شده از CSV واقعی تولید شده‌اند.
- [ ] آخرین PASS و اولین FAIL تکرار شده‌اند؛ تست soak انجام شده است.
- [ ] تحلیل ۱۰k بر اساس نخستین نقطهٔ اشباع اندازه‌گیری‌شده ارائه شده است.
- [ ] اجرای تست روی Docker موجود موجب حذف یا تخریب غیرعمدی Volume نشده است.

---

**یادآوری:** سند حاضر «برنامهٔ اجرای تست» است. آستانه‌های پیشنهادی در بخش ۵ برای قرارداد اجرا هستند و **نتیجهٔ اندازه‌گیری‌شده یا ادعای ظرفیت سخت‌افزار شما نیستند**.
