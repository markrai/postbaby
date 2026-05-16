function persistTabs(tabs) {
    storageAdapter.setJSON('tabs', tabs);
}

function loadTabs() {
    return storageAdapter.getJSON('tabs') || [];
}

function persistActiveTabId(id) {
    storageAdapter.setItem('activeTabId', id);
}

function loadActiveTabId() {
    return storageAdapter.getItem('activeTabId');
}
