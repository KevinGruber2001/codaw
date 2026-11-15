/**
 * This file can be edited to adjust the ESBuild build process.
 * To reset, delete this file and rerun theia build again.
 */
import { __dirname, browserOptions, join, watch } from './gen-esbuild.browser.mjs';

import esbuild from 'esbuild';
import fs from 'node:fs';
import { nodeOptions } from './gen-esbuild.node.mjs';

/**
 * Serve the application icon as the favicon of the browser build: the generated `index.html` has no icon
 * of its own, so the file is copied next to it and a link tag is added once the bundle is written.
 */
browserOptions.plugins.push({
    name: 'codaw-favicon',
    setup(build) {
        build.onEnd(() => {
            const frontend = join(__dirname, 'lib', 'frontend');
            const favicon = join(__dirname, 'ico', 'favicon.ico');
            if (!fs.existsSync(frontend) || !fs.existsSync(favicon)) {
                return;
            }
            fs.copyFileSync(favicon, join(frontend, 'favicon.ico'));
            const indexPath = join(frontend, 'index.html');
            if (fs.existsSync(indexPath)) {
                const html = fs.readFileSync(indexPath, 'utf8');
                if (!html.includes('rel="icon"')) {
                    fs.writeFileSync(indexPath, html.replace(
                        '</head>',
                        '  <link rel="icon" type="image/x-icon" href="./favicon.ico">\n</head>'
                    ));
                }
            }
        });
    }
});

const browserContext = await esbuild.context(browserOptions);
const nodeContext = await esbuild.context(nodeOptions);


if (watch) {
    await Promise.all([
        browserContext.watch(),
        nodeContext.watch(),
    ]);
} else {
    try {
        await browserContext.rebuild();
        await browserContext.dispose();
        await nodeContext.rebuild();
        await nodeContext.dispose();
    } catch {
        process.exit(1);
    }
}
