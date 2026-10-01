const { chromium } = require('playwright');
const fs = require('fs'), path = require('path');
(async () => {
  const xml = fs.readFileSync(process.argv[2], 'utf8');
  const outDir = process.argv[3];
  const browser = await chromium.launch(process.env.CHROME_PATH ? { executablePath: process.env.CHROME_PATH } : {});
  const page = await browser.newPage({ deviceScaleFactor: 2 });
  page.on('console', m => console.log('console:', m.text()));
  page.on('pageerror', e => console.log('err:', e.message));
  const mxdir = path.resolve(__dirname, 'node_modules/mxgraph/javascript/src');
  await page.setContent(`<html><body style="margin:0;background:#fff"><div id="g" style="position:relative;overflow:hidden"></div></body></html>`);
  await page.addScriptTag({ content: `window.mxBasePath='file://${mxdir}';window.mxLoadResources=false;window.mxLoadStylesheet=false;window.mxImageBasePath='file://${mxdir}/images';` });
  await page.addScriptTag({ path: path.join(__dirname, 'node_modules/mxgraph/javascript/mxClient.min.js') });
  const names = await page.evaluate((xml) => {
    const doc = mxUtils.parseXml(xml);
    return Array.from(doc.getElementsByTagName('diagram')).map(d => d.getAttribute('name'));
  }, xml);
  for (let i = 0; i < names.length; i++) {
    const box = await page.evaluate(([xml, i]) => {
      const el = document.getElementById('g'); el.innerHTML = '';
      el.style.width = '3000px'; el.style.height = '2000px';
      const doc = mxUtils.parseXml(xml.replace(/shape=cylinder3/g, 'shape=cylinder'));
      const d = doc.getElementsByTagName('diagram')[i];
      const model = mxUtils.parseXml(mxUtils.getXml(d.getElementsByTagName('mxGraphModel')[0])).documentElement;
      const graph = new mxGraph(el);
      graph.setHtmlLabels(true);
      new mxCodec(model.ownerDocument).decode(model, graph.getModel());
      const b = graph.getGraphBounds();
      graph.view.setTranslate(-b.x + 20, -b.y + 20);
      el.style.width = (b.width + 40) + 'px'; el.style.height = (b.height + 40) + 'px';
      return { w: b.width + 40, h: b.height + 40 };
    }, [xml, i]);
    const file = path.join(outDir, `page${i + 1}.png`);
    await page.locator('#g').screenshot({ path: file });
    console.log(file, names[i], box);
  }
  await browser.close();
})();
