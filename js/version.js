/** Single source of truth for the app build version (shown in the UI). */
const POSTBABY_VERSION = '2.2.1';

window.POSTBABY_VERSION = POSTBABY_VERSION;

(function applyVersionToDom() {
    const el = document.querySelector('.version');
    if (el) {
        el.innerHTML = '<br> v' + POSTBABY_VERSION;
    }
})();
