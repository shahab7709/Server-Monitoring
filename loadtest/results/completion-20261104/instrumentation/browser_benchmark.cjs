// Uses the installed Playwright module and Chrome; override paths on other hosts.
const {chromium} = require(process.env.PLAYWRIGHT_MODULE || '/usr/share/code/resources/app/node_modules/playwright-core');
const fs = require('fs');
(async () => {
  let interrupted=false;
  process.on('SIGTERM', () => {interrupted=true;});
  const browser = await chromium.launch({executablePath: process.env.CHROME || '/opt/google/chrome/chrome', headless:true, args:['--no-sandbox']});
  const end = Date.now() + Number(process.env.SECONDS)*1000;
  const out = fs.createWriteStream(process.env.OUTPUT, {flags:'a'});
  await Promise.all(Array.from({length:10}, async (_,vu) => {
    const page = await browser.newPage();
    await page.goto(process.env.API_URL);
    await page.evaluate(() => {
      // Instrument the existing renderer, then wait for the next painted frame.
      window.benchDone = false;
      const original = render;
      render = function() { original(); requestAnimationFrame(() => requestAnimationFrame(() => {
        window.benchMs = performance.now() - window.benchStart;
        window.benchDone = true;
      })); };
      document.querySelectorAll('#sensors input').forEach(e => e.checked=true);
    });
    while (Date.now() < end && !interrupted) {
      let row = {time:Date.now()/1000, vu, window:'365d'};
      try {
        await page.evaluate(id => {
          device=id; rangeMs=365*86400000; setDates(); window.benchDone=false;
          window.benchStart=performance.now(); load();
        }, process.env.QUERY_DEVICE);
        await page.waitForFunction(() => window.benchDone || !document.getElementById('error').hidden, {timeout:15000});
        Object.assign(row, await page.evaluate(() => ({ms:window.benchMs, error:!document.getElementById('error').hidden})));
      } catch(e) {row.error=String(e);}
      out.write(JSON.stringify(row)+'\n');
      await page.waitForTimeout(1000);
    }
    await page.close();
  }));
  await browser.close(); out.end();
})().catch(e => {console.error(e);process.exit(1)});
