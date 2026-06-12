/** Single source of truth for the app build version (shown in the UI). */
const POSTBABY_VERSION = '2.2.6';
/** Cache-bust token for shell assets; updated by npm run build:public-js. */
const POSTBABY_ASSET_REVISION = '2c201003375c';

window.POSTBABY_VERSION = POSTBABY_VERSION;
window.POSTBABY_ASSET_REVISION = POSTBABY_ASSET_REVISION;

(function applyVersionToDom() {
    const el = document.querySelector('.version');
    if (el) {
        el.innerHTML = '<br> v' + POSTBABY_VERSION;
    }
})();
