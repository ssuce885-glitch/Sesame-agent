const { chromium } = require('playwright');
(async () => {
  const browser = await chromium.launch({ headless: true });
  const page = await browser.newPage({ viewport: { width: 1440, height: 900 } });
  const pages = [
    { path: '/chat', file: '/tmp/sesame-chat.png' },
    { path: '/runtime', file: '/tmp/sesame-runtime.png' },
    { path: '/usage', file: '/tmp/sesame-usage.png' },
    { path: '/roles', file: '/tmp/sesame-roles.png' },
  ];
  for (const p of pages) {
    await page.goto('http://127.0.0.1:4173' + p.path, { waitUntil: 'load' });
    await page.waitForTimeout(3000);
    await page.screenshot({ path: p.file, fullPage: true });
    console.log('Saved', p.file);
  }
  await browser.close();
})();
