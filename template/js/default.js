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
    var waiting = 0, stopped = false, oldHTML = el.innerHTML, timer = setInterval(function () {
        waiting++;
        el.innerHTML = "<b style='font-family:monospace'>" + "|/-\\".charAt(waiting % 4) + "</b>";
    }, 100);
    return function() {
        if (stopped) return;
        stopped = true;
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
        div.style.opacity = "0.9";
        div.style.color = "white";
        div.style.lineHeight = "32px";
        div.style.textAlign = "center";
        div.style.wordBreak = "break-all";
        if (cbres == 'ok') {
            div.style.background = '#088';
            div.innerHTML = '<i class=icon-ok-circled></i>成功';
        } else if (cbres.match(/^ok:/)) {
            div.style.background = '#088';
            div.innerHTML = '<i class=icon-ok-circled></i>' + cbres.substring(3);
        } else {
            div.style.background = "#f52";
            div.innerHTML = '<i class=icon-cancel-circled-1></i>' + (cbres || ("错误状态: " + xml.status));
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
        a.onclick = (function(i, img) {
            return function() {
                img.src = '/s/css/spinner.gif';
                $post('/api/user_kimochi', {k: i}, function(resp) {
                    if (resp === 'ok')  location.reload(); 
                });
            }
        })(i, img)
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
            num.innerText = (parseInt(num.innerText) || 0) + 1;
        } else {
            el.setAttribute("liked", "false")
            icon.className = 'icon-heart-1';
            num.innerText = parseInt(num.innerText) ? (parseInt(num.innerText) - 1) : 0;
        }
    }, stop);
}

function deleteArticle(el, id) {
    if (!confirm("是否确认删除该发言？该操作不可逆")) return;
    var stop = $wait(el);
    $post("/api2/new", { parent: id, delete: 1 }, function (res) {
        stop();
        if (res != "ok") return res;
        $q("[data-id='" + id + "'] > pre", true).forEach(function(e) {
            e.innerHTML = "<span class=deleted></span>";
        });
        $q("[data-id='" + id + "'] img", true).forEach(function(e) {
            e.src = '';
        });
    }, stop)
}

function nsfwArticle(el, id) {
    var stop = $wait(el);
    $post("/api2/new", { parent: id, makensfw: 1 }, function (res) {
        stop();
        if (res != "ok") return res;
        el.setAttribute("value", !($value(el) === 'true'))
        el.style.color = $value(el) === 'true' ? "#f90" : "#bbb"
        return "ok";
    }, stop);
}

function followBlock(el, m, id) {
    var stop = $wait(el), obj = { method: m };
    obj[m] = $value(el) === "true" ? "" : "1";
    obj['to'] = id;
    el.setAttribute("value", obj[m] == "" ? "false" : "true");
    $post("/api2/follow_block", obj, function(res) {
        stop();
        if (res != "ok") return res;
        if (m == "follow") {
            el.innerHTML = '<i class=' + ((obj[m] != "") ? "icon-heart-broken" : "icon-user-plus") + "></i>";
            return "ok:" + ((obj[m] != "") ? "已关注" + id : "已取消关注" + id);
        } else {
            el.style.color = (obj[m] != "") ? "#f52" : "#aaa";
            return "ok:" + ((obj[m] != "") ? "已拉黑" + id : "已解除" + id + "拉黑状态")
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

function loadMore(tlid, el, data) {
    data.cursors = $value(el);
    var stop = $wait(el);
    $post('/api/timeline', data, function(pl) {
        stop();
        if (pl.EOT) {
            el.innerText = "没有更多内容了";
            el.setAttribute("eot", "true");
            el.style.color = "#aaa";
            el.onclick = function() { location.reload() }
        } else {
            el.innerText = "更多...";
        }
        if (pl.Articles) {
            el.setAttribute("value", pl.Next);
            pl.Articles.forEach(function(a) {
                var dedup = $q('#' + tlid + " > [data-id='" + a[0] + "']");
                if (dedup && dedup.length) {
                    console.log("dedup:", a[0])
                    return;
                }
                var div = document.createElement("div");
                div.innerHTML = a[1];
                $q('#' + tlid).appendChild(div.firstChild);
            })
        }
        expandNSFW();
        if (!data.reply)
            history.pushState("", "", location.pathname + location.search)
    }, stop);
    //   console.log(document.documentElement.scrollTop);
}

function preLoadMore(tlid, nextBtn) {
    window.addEventListener('scroll', function(e) {
        if (!window.ticking) {
            window.requestAnimationFrame(function() {
                $q("#" + tlid + " > .row", true).forEach(function(c) {
                    if (isInViewport(c, 3)) {
                        if (c.childNodes.length == 0) {
                            c.innerHTML = c.__html;
                            c.style.height = "";
                        }
                    } else {
                        if (c.childNodes.length) {
                            c.style.height = c.offsetHeight + "px";
                            c.__html = c.innerHTML;
                            c.innerHTML = "";
                        }
                    }
                })
                if (isInViewport(nextBtn) &&
                    !nextBtn.getAttribute("disabled") && nextBtn.getAttribute("eot") !== "true") {
                    console.log("Load next");
                    nextBtn.click();
                }
                window.ticking = false;
            });
            window.ticking = true;
        }
    });
}

// Nested replies view
function showReply(aid) {
    var div = $q('<div>');
    div.id = 'Z' + Math.random().toString(36).substr(2, 5);
    div.className = 'div-inner-reply';
    div.style.position = 'fixed';
    div.style.left = '0';
    div.style.top = '0';
    div.style.width = '100%';
    div.style.height = '100%';
    div.style.backgroundColor = 'white';
    div.style.overflowY = 'scroll';
    div.style.overflowX = 'hidden';
    div.style.backgroundImage = 'url(/s/css/spinner.gif)';
    div.style.backgroundRepeat = 'no-repeat';
    div.style.backgroundPosition = 'center center';
    $post('/api/p/' + aid, {}, function(h) {
        div.innerHTML = h;
        div.style.backgroundImage = null;
    });

    $q("[data-parent='" + aid + "']", true).forEach(function(e) {
        e.CLOSER.click();
    });
    div.setAttribute('data-parent', aid);

    var divreload = $q("<div>");
    divreload.style.position = 'fixed';
    divreload.style.right = '1em';
    divreload.style.top = '3.5em';
    divreload.innerHTML = "<i class='control icon-cw-circled'></i>"
    divreload.onclick = function() { showReply(aid) }

    var divclose = $q("<div>");
    divclose.style.position = 'fixed';
    divclose.style.right = '1em';
    divclose.style.top = '1em';
    divclose.innerHTML = "<i class='control icon-cancel-circled-1'></i>"
    divclose.onclick = function() {
        div.parentNode.removeChild(div)
        divclose.parentNode.removeChild(divclose)
        divreload.parentNode.removeChild(divreload)

        if ($q('[data-parent]', true).length === 0) {
            history.pushState("", "", location.pathname + (window.IS_MEDIA ? '?media=1' : ''));
            document.body.style.overflow = null;
        }
    }
    div.CLOSER = divclose;

    document.body.appendChild(div);
    document.body.appendChild(divclose);
    document.body.appendChild(divreload);
    document.body.style.overflow = 'hidden';

    window.IS_MEDIA = window.IS_MEDIA || location.search.indexOf('media') >= 0;
    history.pushState("", "", location.pathname + "?pid=" + encodeURIComponent(aid) + location.hash + "#" + div.id)
}

window.onpopstate = function(event) {
    var closes = $q(".div-inner-reply", true)
    location.href.split("#").forEach(function(id) {
        closes = closes.filter(function(c) { return c.id != id })
    })
    closes.forEach(function(c) { c.CLOSER.click() })
};

function updateSetting(el, field, value) {
    var data = {}, stop = $wait(el.tagName === 'BUTTON' ? el : el.nextElementSibling)
    data["set-" + field] = "1";
    if (field !== 'bio') {
        data[field] = value;
    } else {
        ["description"].forEach(function(id) { data[id] = $q("[name='" + id + "']").value })
    }
    $post("/api/user_settings", data, function(h) { return h }, stop)
}
