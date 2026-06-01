/*
 * Regression self-check for the public JS build workflow.
 *
 * Proves that `build:public-js` repairs a stale service-worker
 * PUBLIC_JS_REVISION even when js/script.js content does not change, so
 * maintainers never have to delete js/script.js to force a revision repair.
 *
 * Scenario:
 *   1. Build once so js/script.js + sw.js are current.
 *   2. Snapshot js/script.js.
 *   3. Deliberately corrupt sw.js with a stale PUBLIC_JS_REVISION.
 *   4. Run build:public-js.
 *   5. Assert js/script.js is byte-for-byte unchanged.
 *   6. Assert sw.js revision was repaired to the expected value.
 *   7. Run verify:public-js (--check) and assert it passes.
 */

import crypto from 'node:crypto';
import fs from 'node:fs/promises';
import path from 'node:path';
import process from 'node:process';
import { spawnSync } from 'node:child_process';
import { fileURLToPath } from 'node:url';

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);
const repoRoot = path.resolve(__dirname, '..');

const buildScript = path.join(__dirname, 'build-public-js.mjs');
const publicPath = path.join(repoRoot, 'js', 'script.js');
const serviceWorkerPath = path.join(repoRoot, 'sw.js');
const revisionPattern = /const PUBLIC_JS_REVISION = '([0-9a-f]+)';/;
const STALE_REVISION = 'badc0ffee000';

function runBuild(extraArgs = []) {
    const result = spawnSync(process.execPath, [buildScript, ...extraArgs], {
        cwd: repoRoot,
        encoding: 'utf8',
        windowsHide: true
    });

    if (result.status !== 0) {
        const output = `${result.stdout || ''}${result.stderr || ''}`.trim();
        throw new Error(
            `build-public-js.mjs ${extraArgs.join(' ')} exited with code ${result.status}.\n${output}`
        );
    }

    return result;
}

function sha256(text) {
    return crypto.createHash('sha256').update(text).digest('hex');
}

function readRevision(serviceWorkerText) {
    const match = serviceWorkerText.match(revisionPattern);
    if (!match) {
        throw new Error('sw.js is missing the PUBLIC_JS_REVISION token.');
    }
    return match[1];
}

async function main() {
    console.log('[self-check] Step 1: build once so artifacts are current.');
    runBuild();

    const jsBefore = await fs.readFile(publicPath, 'utf8');
    const swBefore = await fs.readFile(serviceWorkerPath, 'utf8');
    const expectedRevision = readRevision(swBefore);
    const jsBeforeHash = sha256(jsBefore);

    if (expectedRevision === STALE_REVISION) {
        throw new Error('Unexpected: real revision collides with the synthetic stale token.');
    }

    console.log(`[self-check] Step 2/3: corrupt sw.js revision ${expectedRevision} -> ${STALE_REVISION}.`);
    const corruptedServiceWorker = swBefore.replace(
        revisionPattern,
        `const PUBLIC_JS_REVISION = '${STALE_REVISION}';`
    );
    if (corruptedServiceWorker === swBefore) {
        throw new Error('Failed to inject a stale revision into sw.js.');
    }
    await fs.writeFile(serviceWorkerPath, corruptedServiceWorker, 'utf8');

    try {
        console.log('[self-check] Step 4: run build:public-js with js/script.js unchanged.');
        runBuild();

        const jsAfter = await fs.readFile(publicPath, 'utf8');
        const swAfter = await fs.readFile(serviceWorkerPath, 'utf8');
        const repairedRevision = readRevision(swAfter);

        console.log('[self-check] Step 5: assert js/script.js is unchanged.');
        if (sha256(jsAfter) !== jsBeforeHash) {
            throw new Error('js/script.js content changed during a revision-only repair.');
        }

        console.log('[self-check] Step 6: assert sw.js revision was repaired.');
        if (repairedRevision === STALE_REVISION) {
            throw new Error('sw.js still contains the stale revision; repair did not run.');
        }
        if (repairedRevision !== expectedRevision) {
            throw new Error(
                `sw.js revision was repaired to ${repairedRevision}, expected ${expectedRevision}.`
            );
        }

        console.log('[self-check] Step 7: run verify:public-js (--check).');
        runBuild(['--check']);

        console.log(`[self-check] PASS: stale sw.js revision repaired to ${repairedRevision} without rebuilding js/script.js.`);
    } catch (error) {
        // Best-effort restoration so a failed run does not leave a corrupted sw.js.
        try {
            await fs.writeFile(serviceWorkerPath, swBefore, 'utf8');
        } catch {
            // Ignore restore failures; the original error is more important.
        }
        throw error;
    }
}

main().catch((error) => {
    console.error(`[self-check] FAIL: ${error instanceof Error ? error.message : String(error)}`);
    process.exitCode = 1;
});
