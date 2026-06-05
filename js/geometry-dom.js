(function () {
    function getPointerClientPoint(event) {
        if (event.type.startsWith('touch')) {
            return {
                x: event.touches[0].clientX,
                y: event.touches[0].clientY
            };
        }

        return {
            x: event.clientX,
            y: event.clientY
        };
    }

    function getPointerPagePoint(event) {
        if (event.type.startsWith('touch')) {
            return {
                x: event.touches[0].pageX,
                y: event.touches[0].pageY
            };
        }

        return {
            x: event.pageX,
            y: event.pageY
        };
    }

    function getContainerRelativePointFromClient(clientX, clientY, containerElement) {
        const containerRect = containerElement.getBoundingClientRect();
        return {
            x: clientX - containerRect.left,
            y: clientY - containerRect.top
        };
    }

    function getContainerRelativePositionStringsFromPage(pageX, pageY, containerElement) {
        const containerRect = containerElement.getBoundingClientRect();
        const containerLeft = containerRect.left + window.scrollX;
        const containerTop = containerRect.top + window.scrollY;
        return {
            top: (pageY - containerTop) + 'px',
            left: (pageX - containerLeft) + 'px'
        };
    }

    function getElementClientRect(element) {
        return element.getBoundingClientRect();
    }

    function getElementOffsetRect(element) {
        return {
            left: element.offsetLeft,
            top: element.offsetTop,
            width: element.offsetWidth,
            height: element.offsetHeight,
            right: element.offsetLeft + element.offsetWidth,
            bottom: element.offsetTop + element.offsetHeight
        };
    }

    function getElementOffsetCenter(element) {
        const rect = getElementOffsetRect(element);
        return {
            cx: rect.left + rect.width / 2,
            cy: rect.top + rect.height / 2
        };
    }

    function buildClientRectFromPoints(startClientPoint, endClientPoint) {
        const left = Math.min(startClientPoint.x, endClientPoint.x);
        const top = Math.min(startClientPoint.y, endClientPoint.y);
        const right = Math.max(startClientPoint.x, endClientPoint.x);
        const bottom = Math.max(startClientPoint.y, endClientPoint.y);
        return {
            left: left,
            top: top,
            right: right,
            bottom: bottom,
            width: right - left,
            height: bottom - top
        };
    }

    function clientRectsIntersect(a, b) {
        return !(
            a.top > b.bottom
            || a.right < b.left
            || a.bottom < b.top
            || a.left > b.right
        );
    }

    window.PostbabyGeometryDom = Object.freeze({
        getPointerClientPoint,
        getPointerPagePoint,
        getContainerRelativePointFromClient,
        getContainerRelativePositionStringsFromPage,
        getElementClientRect,
        getElementOffsetRect,
        getElementOffsetCenter,
        buildClientRectFromPoints,
        clientRectsIntersect
    });
})();
