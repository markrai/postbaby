/*
 * Regression self-check for the public JS build workflow.
 *
 * Proves that `build:public-js`:
 *   1. repairs a stale service-worker PUBLIC_JS_REVISION even when
 *      js/script.js content does not change,
 *   2. rejects service workers with zero PUBLIC_JS_REVISION definitions,
 *   3. rejects merge-conflicted service workers,
 *   4. repairs malformed sw.js back to the clean generated artifact.
 */

import crypto from 'node:crypto';
import fs from 'node:fs/promises';
import os from 'node:os';
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
const revisionPattern = /const PUBLIC_JS_REVISION = '([0-9a-f]+)';/g;
const conflictMarkerPattern = /^(<<<<<<<|=======|>>>>>>>)(?: .*)?$/m;
const STALE_REVISION = 'badc0ffee000';

function combinedOutput(result) {
    return `${result.stdout || ''}${result.stderr || ''}`.trim();
}

function runBuild(extraArgs = [], options = {}) {
    const result = spawnSync(process.execPath, [buildScript, ...extraArgs], {
        cwd: repoRoot,
        encoding: 'utf8',
        windowsHide: true,
        env: {
            ...process.env,
            ...(options.env || {})
        }
    });
    const output = combinedOutput(result);

    if (options.expectFailure) {
        if (result.status === 0) {
            throw new Error(
                `build-public-js.mjs ${extraArgs.join(' ')} unexpectedly succeeded.\n${output}`
            );
        }

        return { result, output };
    }

    if (result.status !== 0) {
        throw new Error(
            `build-public-js.mjs ${extraArgs.join(' ')} exited with code ${result.status}.\n${output}`
        );
    }

    return { result, output };
}

function sha256(text) {
    return crypto.createHash('sha256').update(text).digest('hex');
}

function readSingleRevision(serviceWorkerText) {
    const matches = [...serviceWorkerText.matchAll(revisionPattern)];
    if (matches.length !== 1) {
        throw new Error(`sw.js must contain exactly one PUBLIC_JS_REVISION definition; found ${matches.length}.`);
    }
    return matches[0][1];
}

function assertNoConflictMarkers(serviceWorkerText, label) {
    if (conflictMarkerPattern.test(serviceWorkerText)) {
        throw new Error(`${label} still contains merge conflict markers.`);
    }
}

async function assertCheckFailsForMalformedServiceWorker(serviceWorkerText, expectedSnippet, label, buildEnv) {
    await fs.writeFile(serviceWorkerPath, serviceWorkerText, 'utf8');
    const { output } = runBuild(['--check'], { expectFailure: true, env: buildEnv });
    if (!output.includes(expectedSnippet)) {
        throw new Error(`${label} verify output did not mention "${expectedSnippet}".\n${output}`);
    }

    runBuild([], { env: buildEnv });

    const repairedServiceWorker = await fs.readFile(serviceWorkerPath, 'utf8');
    assertNoConflictMarkers(repairedServiceWorker, label);
    return readSingleRevision(repairedServiceWorker);
}

async function main() {
    console.log('[self-check] Step 1: build once so artifacts are current.');
    runBuild();

    const jsBefore = await fs.readFile(publicPath, 'utf8');
    const swBefore = await fs.readFile(serviceWorkerPath, 'utf8');
    const expectedRevision = readSingleRevision(swBefore);
    const jsBeforeHash = sha256(jsBefore);
    const tempDir = await fs.mkdtemp(path.join(os.tmpdir(), 'postbaby-sw-self-check-'));
    const cleanTemplatePath = path.join(tempDir, 'sw.template.js');
    const repairEnv = {
        POSTBABY_PRIVATE_SERVICE_WORKER_SOURCE: cleanTemplatePath
    };
    await fs.writeFile(cleanTemplatePath, swBefore, 'utf8');

    if (expectedRevision === STALE_REVISION) {
        throw new Error('Unexpected: real revision collides with the synthetic stale token.');
    }

    console.log(`[self-check] Step 2/3: corrupt sw.js revision ${expectedRevision} -> ${STALE_REVISION}.`);
    const staleRevisionServiceWorker = swBefore.replace(
        /const PUBLIC_JS_REVISION = '[0-9a-f]+';/,
        `const PUBLIC_JS_REVISION = '${STALE_REVISION}';`
    );
    if (staleRevisionServiceWorker === swBefore) {
        throw new Error('Failed to inject a stale revision into sw.js.');
    }
    await fs.writeFile(serviceWorkerPath, staleRevisionServiceWorker, 'utf8');

    try {
        console.log('[self-check] Step 4: run build:public-js with js/script.js unchanged.');
        runBuild([], { env: repairEnv });

        const jsAfterRevisionRepair = await fs.readFile(publicPath, 'utf8');
        const swAfterRevisionRepair = await fs.readFile(serviceWorkerPath, 'utf8');
        const repairedRevision = readSingleRevision(swAfterRevisionRepair);

        console.log('[self-check] Step 5: assert js/script.js is unchanged.');
        if (sha256(jsAfterRevisionRepair) !== jsBeforeHash) {
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

        console.log('[self-check] Step 7: inject sw.js with zero PUBLIC_JS_REVISION definitions.');
        const zeroRevisionServiceWorker = swBefore.replace(
            /const PUBLIC_JS_REVISION = '[0-9a-f]+';/,
            `const PUBLIC_JS_CACHE_REVISION = '${expectedRevision}';`
        );
        const zeroRevisionRepair = await assertCheckFailsForMalformedServiceWorker(
            zeroRevisionServiceWorker,
            'exactly one PUBLIC_JS_REVISION definition; found 0',
            'Zero-definition sw.js',
            repairEnv
        );
        if (zeroRevisionRepair !== expectedRevision) {
            throw new Error(
                `Zero-definition sw.js repaired to ${zeroRevisionRepair}, expected ${expectedRevision}.`
            );
        }

        console.log('[self-check] Step 8: inject merge-conflict markers into sw.js.');
        const conflictedServiceWorker = swBefore.replace(
            /const PUBLIC_JS_REVISION = '[0-9a-f]+';/,
            `<<<<<<< HEAD\nconst PUBLIC_JS_REVISION = '${expectedRevision}';\n=======\nconst PUBLIC_JS_REVISION = '${STALE_REVISION}';\n>>>>>>> synthetic`
        );
        const conflictRepair = await assertCheckFailsForMalformedServiceWorker(
            conflictedServiceWorker,
            'merge conflict markers',
            'Conflicted sw.js',
            repairEnv
        );
        if (conflictRepair !== expectedRevision) {
            throw new Error(
                `Conflicted sw.js repaired to ${conflictRepair}, expected ${expectedRevision}.`
            );
        }

        console.log('[self-check] Step 9: run verify:public-js (--check).');
        runBuild(['--check'], { env: repairEnv });

        console.log(`[self-check] PASS: malformed sw.js artifacts were rejected and repaired back to revision ${expectedRevision} without rebuilding js/script.js.`);
    } catch (error) {
        try {
            await fs.writeFile(serviceWorkerPath, swBefore, 'utf8');
        } catch {
            // Ignore restore failures; the original error is more important.
        }
        throw error;
    } finally {
        await fs.rm(tempDir, { recursive: true, force: true });
    }
}

main().catch((error) => {
    console.error(`[self-check] FAIL: ${error instanceof Error ? error.message : String(error)}`);
    process.exitCode = 1;
});
