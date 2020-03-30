"use strict";

window.z = function (q, asarray) {
    var el = document.querySelectorAll(q), ela = [];
    if (el.length === 1 && asarray !== true) return el[0];
    for (var i = 0; i < el.length; i++) ela.push(el[i]);
    return ela;
}

window.z.html = function (h) {
    var div = document.createElement("div")
    div.innerHTML = h;
    return div.firstElementChild
}

window.z.attr = function (el, attr, value) {
    return el && el.getAttribute && (
        value !== undefined ? el.setAttribute(attr, value) : el.getAttribute(attr))
}

window.z.wait = function (el) {
    var waiting = 0,
        stopped = false,
        oldHTML = el.innerHTML,
        specialClass = el.className.match(/icon-\S+/g),
        timer = setInterval(function () {
            waiting++;
            el.innerHTML = "<b style='font-family:monospace;font-size:inherit'>" + "|/-\\".charAt(waiting % 4) + "</b>";
        }, 100);

    el.setAttribute("disabled", "disabled");
    el.className = el.className.replace(/icon-\S+/, '');
    return function() {
        if (stopped) return;
        stopped = true;
        el.removeAttribute("disabled");
        clearInterval(timer);
        el.innerHTML = oldHTML;
        el.className += (specialClass || []).join('');
    }
}
