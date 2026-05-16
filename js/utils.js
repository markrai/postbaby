const isMobileDeviceForSure = () => {
    const isTouch = 'ontouchstart' in window || navigator.maxTouchPoints > 0;
    const isMobileUA = /Mobi|Android|iPhone|iPad|iPod|Windows Phone/i.test(navigator.userAgent);
    const isSmallScreen = window.innerWidth <= 768; // Optional: Screen width check

    // Consider it mobile if it's a touch device with a mobile user agent or small screen
    return isTouch && isMobileUA && isSmallScreen;
};

function isTouchDevice() {
    return 'ontouchstart' in window || navigator.maxTouchPoints > 0 && /Mobi|Android|iPhone|iPad|iPod|Windows Phone/i.test(navigator.userAgent);
}


function generateUUID() {
    var d = new Date().getTime(); 
    var d2 = (performance && performance.now && (performance.now() * 1000)) || 0;
    return 'xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx'.replace(/[xy]/g, function (c) {
        var r = Math.random() * 16; 
        if (d > 0) {
            r = (d + r) % 16 | 0;
            d = Math.floor(d / 16);
        } else {
            r = (d2 + r) % 16 | 0;
            d2 = Math.floor(d2 / 16);
        }
        return (c == 'x' ? r : (r & 0x3 | 0x8)).toString(16);
    });
}


function showToast(message) {
    // Create toast element
    let toast = document.createElement('div');
    toast.className = 'toast';
    toast.innerText = message;

    // Append to body
    document.body.appendChild(toast);

    // Trigger reflow to restart CSS animations
    void toast.offsetWidth; // Forces reflow

    // Add a class to start the animations
    toast.classList.add('show');

    // Remove after 3 seconds
    setTimeout(() => {
        toast.remove();
    }, 1000);
}


function isEditingText() {
    const activeElement = document.activeElement;
    return activeElement.tagName === 'TEXTAREA' ||
        activeElement.tagName === 'INPUT' ||
        activeElement.isContentEditable;
}


function capitalizeFirstLetter(string) {
    
    if (string.toLowerCase() === "nowlater") {
        return "Now & Later";
    }

    if (string.toLowerCase() === "eisenhower") {
        return "Eisenhower Matrix";
    }

    if (string.toLowerCase() === "smartgoals") {
        return "S.M.A.R.T Goals";
    }

    if (string.toLowerCase() === "swot") {
        return "S.W.O.T Analysis";
    }

    if (string.toLowerCase() === "kanban") {
        return "Kanban Board";
    }

    if (string.toLowerCase() === "priority") {
        return "Priority Matrix";
    }

    // Default capitalization logic
    return string.charAt(0).toUpperCase() + string.slice(1);
}

function showMobileNotification() {
    const notification = document.getElementById('mobileNotification');
    if (isMobileDeviceForSure() && notification) {
        notification.style.display = 'block';
    } else if (notification) {
        notification.style.display = 'none';
    }
}


function isIOS() {
    const userAgent = navigator.userAgent || navigator.vendor || window.opera;
    const platform = navigator.platform || "";

    const isIOSDevice = /iPhone|iPad|iPod/i.test(userAgent);

    const isMacLike = platform.includes("Mac") && navigator.maxTouchPoints > 1;

    return isIOSDevice || isMacLike;
}


function showIOSNotification() {
    const notification = document.getElementById('iosnotification');
    const okButton = document.getElementById('iosOkButton');

    if (isIOS() && notification) {
        notification.style.display = 'flex'; // Use flex for centering content
    }

    // Add event listener for the OK button
    if (okButton) {
        okButton.addEventListener('click', () => {
            notification.style.display = 'none';
        });
    }
}


function isModalOpen() {
    const modals = document.querySelectorAll('.modal');
    for (let modal of modals) {
        if (window.getComputedStyle(modal).display !== 'none') {
            return true;
        }
    }
    return false;
}