function $q(q, multi) {
    if (q.match(/^<\S+>$/)) return document.createElement(q.substring(1, q.length - 1));
    var el = document.querySelectorAll(q), ela = [];
    if (el.length === 1 && !multi) return el[0];
    for (var i = 0; i < el.length; i++) ela.push(el[i]);
    return ela;
}

function $value(el) {
    return el.getAttribute("value")
}

function $wait(el) {
    el.setAttribute("disabled", "disabled");
    var waiting = 0,
        oldHTML = el.innerHTML;
    timer = setInterval(function () {
        waiting++;
        el.innerHTML = "<b style='font-family:monospace'>" + "|/-\\".charAt(waiting % 4) + "</b>";
    }, 100);
    return function() {
        el.removeAttribute("disabled");
        clearInterval(timer);
        el.innerHTML = oldHTML;
    }
}

function $post(url, data, cb, errorcb) {
    var xml = new XMLHttpRequest(), m = document.cookie.match(/(^| )id=([^;]+)/);
    var cbres = null;
    xml.onreadystatechange = function() {
        if (xml.readyState != 4) return;
        if (xml.status == 200) {
            var res = xml.responseText;
            if (xml.getResponseHeader('Content-Type').match(/json/)) {
                try { res = JSON.parse(res) } catch(e) {}
            }

            cbres = cb(res)
            if (!cbres) return;

            // callback returns error
            cbres = __i18n(cbres);
        }
        xml.onerror();
    }
    xml.onerror = function() {
        if (errorcb) errorcb(xml)
        var div = $q("<div>");
        div.style.position = "fixed";
        div.style.top = '0'; div.style.left = '0'; div.style.width = "100%";
        div.style.opacity = "0.85";
        div.style.color = "white";
        div.style.lineHeight = "32px";
        div.style.textAlign = "center";
        div.style.wordBreak = "break-all";
        if (cbres == 'ok') {
            div.style.background = '#088';
            div.innerHTML = '<i class=icon-ok-circled></i>成功';
        } else {
            div.style.background = "#f52";
            div.innerHTML = '<i class=icon-cancel-circled></i>' + (cbres || ("错误状态: " + xml.status));
        }
        document.body.appendChild(div);
        setTimeout(function() {
            div.style.transition = "opacity 1s";
            div.style.opacity = "0";
            setTimeout(function() {div.parentNode.removeChild(div)}, 1000);
        }, 1500)
    }
    xml.open("POST", url, true);
    xml.setRequestHeader('Content-Type', 'application/x-www-form-urlencoded');
    var q = "api=1&api2_uid=" + (m ? m[2] : "");
    for (var k in data) {
        if (data.hasOwnProperty(k)) q += '&' + k + '=' + encodeURIComponent(data[k]);
    }
    xml.send(q);
}

function $postReload(el, url, data) {
    var stop = $wait(el);
    $post(url, data, function(res) {
        stop();
        if (res != "ok") return res;
        location.reload();
    }, stop)
}

function loadKimochi(el) {
    var ul = el.querySelector('ul');
    if (ul.childNodes.length) return;

    for (var i = 0; i <= 44; i++) {
        var li = $q('<li>'), a = $q('<a>'), img = $q('<img>');
        img.src = '/s/emoji/emoji' + i + '.png';
        if (i == 0) {
            img.style.border = 'dashed 1px #233';
            img.style.borderRadius = '50%';
        }
        a.appendChild(img);
        a.onclick = (function(i) {
            return function() {
                $post('/api/user_kimochi', {k: i}, function(resp) {
                    if (resp === 'ok')  location.reload(); 
                });
            }
        })(i)
        li.appendChild(a);
        ul.appendChild(li);
    }
}

function isInViewport(el, scale) {
    var top = el.offsetTop, height = el.offsetHeight, h = window.innerHeight, s = scale || 0;
    while (el.offsetParent) {
        el = el.offsetParent;
        top += el.offsetTop;
    }
    return top < (window.pageYOffset + h + h*s) && (top + height) > window.pageYOffset - h*s;
} 

function likeArticle(el, id) {
    var v = el.getAttribute("liked") === "true" ? "" : "1",
        num = el.querySelector('span.num'),
        icon = el.querySelector('i');
    var stop = $wait(num);
    $post("/api2/like_article", {like:(v || ""), to:id}, function(res) {
        stop();
        if (res !== "ok") return res;
        if (v) {
            el.setAttribute("liked", "true")
            icon.className = 'icon-heart-filled';
            num.innerText = parseInt(num.innerText) + 1;
        } else {
            el.setAttribute("liked", "false")
            icon.className = 'icon-heart-1';
            num.innerText = parseInt(num.innerText) ? (parseInt(num.innerText) - 1) : 0;
        }
    }, stop);
}

function followBlock(el, m, id) {
    var stop = $wait(el), obj = { method: m };
    obj[m] = $value(el);
    obj['to'] = id;
    el.setAttribute("value", obj[m] != "" ? "" : "1");
    $post("/api2/follow_block", obj, function(res) {
        stop();
        if (res != "ok") return res;
        if (m == "follow") {
            el.innerText = (obj[m] != "") ? "取消关注" : "关注";
        } else {
            el.innerText = (obj[m] != "") ? "解除黑名单" : "拉入黑名单";
        }
    }, stop)
}

function __i18n(t) {
    if (t.match(/guard\/cooling-down\/([0-9\.]+)s/)) 
        return "请等待" + t.split("/").pop();
    if (t === "guard/failed-captcha")
        return "无效验证码";
    if (t === "guard/token-expired")
        return "Token过期，请重试";
    if (t === "content/too-short")
        return "正文过短";
    if (t === "title/too-short")
        return "标题过短";
    if (t === "error/can-not-reply")
        return "无法回复";
    if (t === "internal/error")
        return "服务端异常";
    if (t === "guard/id-not-existed")
        return "ID不存在";
    if (t === "user/not-logged-in")
        return "请登入后操作";
    if (t === "password/invalid-too-short")
        return "密码太短或不符";
    if (t === "id/already-existed")
        return "ID已存在";
    if (t === "id/too-short")
        return "无效ID";
    return t;
}
