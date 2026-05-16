function findTab(tabs, tabId) {
    return tabs.find(t => t.id === tabId);
}

function getActiveTab(tabs, activeTabId) {
    return tabs.find(t => t.id === activeTabId);
}

function findItem(tab, itemId) {
    return tab.items.find(it => it.id === itemId);
}
