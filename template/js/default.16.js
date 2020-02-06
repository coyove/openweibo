function $q(q, multi) {
    if (q.match(/^<\S+>$/)) return document.createElement(q.substring(1, q.length - 1));
    var el = document.querySelectorAll(q), ela = [];
    if (el.length === 1 && !multi) return el[0];
    for (var i = 0; i < el.length; i++) ela.push(el[i]);
    return ela;
}

function $html(h) {
    var div = $q("<div>")
    div.innerHTML = h;
    return div.firstElementChild
}

function $value(el) {
    return el && el.getAttribute && el.getAttribute("value")
}

function $wait(el) {
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

function $popup(html, bg) {
    var div = $html("<div style='position:fixed;top:0;left:0;width:100%;color:white;opacity:0.9;line-height:32px;text-align:center;word-break:break-all'></div>");
    div.style.background = bg || '#f52';
    div.innerHTML = html;
    document.body.appendChild(div);
    setTimeout(function() {
        div.style.transition = "opacity 1s";
        div.style.opacity = "0";
        setTimeout(function() {div.parentNode.removeChild(div)}, 1000);
    }, 1000)
}

function $post(url, data, cb, errorcb) {
    var xml = new XMLHttpRequest(), m = document.cookie.match(/(^| )id=([^;]+)/);
    var cbres = null;
    xml.onreadystatechange = function() {
        if (xml.readyState != 4) return;
        if (xml.status == 200) {
            var res = xml.responseText;
            if ((xml.getResponseHeader('Content-Type') || "").match(/json/)) {
                try { res = JSON.parse(res) } catch(e) {}
            }

            cbres = cb(res, xml)
            if (!cbres) return;

            // callback returns error
            cbres = __i18n(cbres);
        }
        xml.onerror();
    }
    xml.onerror = function() {
        if (cbres == 'ok') {
            $popup('<i class=icon-ok-circled></i>成功', '#088');
        } else if (cbres && cbres.match(/^ok:/)) {
            $popup('<i class=icon-ok-circled></i>' + cbres.substring(3), '#088');
        } else {
            if (errorcb) errorcb(xml)
            $popup('<i class=icon-cancel-circled-1></i>' + (cbres || ("错误状态: " + xml.status)));
        }
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
    $post("/api2/delete", { id: id }, function (res) {
        stop();
        if (res != "ok") return res;
        $q("[data-id='" + id + "'] > pre", true).forEach(function(e) {
            e.innerHTML = "<span class=deleted></span>";
        });
        $q("[data-id='" + id + "'] .media img", true).forEach(function(e) {
            e.src = '';
        });
        $q("[data-id='" + id + "'] .media", true).forEach(function(e) {
            e.style.display = 'none';
        });
    }, stop)
}

function nsfwArticle(el, id) {
    var stop = $wait(el);
    $post("/api2/toggle_nsfw", { id: id }, function (res) {
        stop();
        if (res != "ok") return res;
        el.setAttribute("value", !($value(el) === 'true'))
        el.style.color = $value(el) === 'true' ? "#f90" : "#bbb"
        return "ok";
    }, stop);
}

function lockArticle(el, id) {
    var stop = $wait(el);
    $post("/api2/toggle_lock", { id: id }, function (res) {
        stop();
        if (res != "ok") return res;

        var locked = $value(el) !== 'true';
        el.setAttribute("value", locked)
        el.querySelector("i").className = locked ? "icon-lock" : "icon-lock-open"
        el.querySelector("i").style.color = locked ? "#233" : "#aaa"
        return "ok:" + (locked ? "已锁定该状态，其他人不能回复" : "已解除锁定");
    }, stop);
}

function followBlock(el, m, id) {
    if (m == "block" && $value(el) === "false") {
        if (window.localStorage.getItem('not-first-block') != 'true') {
            if (!confirm("是否确定拉黑" + id)) {
                return;
            }
            window.localStorage.setItem('not-first-block', 'true')
        }
    }
    var stop = $wait(el), obj = { method: m };
    id = id || el.getAttribute("user-id");
    obj[m] = $value(el) === "true" ? "" : "1";
    obj['to'] = id;
    $post("/api2/follow_block", obj, function(res) {
        stop();
        if (res != "ok") return res;
        el.setAttribute("value", obj[m] == "" ? "false" : "true");
        if (m == "follow") {
            el.innerHTML = '<i class=' + ((obj[m] != "") ? "icon-heart-broken" : "icon-user-plus") + "></i>";
            return "ok:" + ((obj[m] != "") ? "已关注" + id : "已取消关注" + id);
        } else {
            el = el.querySelector('i');
            el.className = el.className.replace(/block-\S+/, '') + " block-" + (obj[m] != "");
            el = el.nextElementSibling;
            if (el) el.innerText = obj[m] != "" ? "解除" : "拉黑";
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
    if (t === "user/not-allowed")
        return "无权限";
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
                $q('#' + tlid).appendChild(div.querySelector("div"));
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

    // Close duplicated windows before opening a new one
    $q("[data-parent='" + aid + "']", true).forEach(function(e) { e.CLOSER.click(); });
    div.setAttribute('data-parent', aid);

    var divclose = $html("<div style='margin:0 auto' class='rows replies'><div class=row style='padding:0.5em;line-height:30px;display:flex'>" +
        "<i class='control icon-left-small'></i>" + 
        "<input style='margin:0 0.5em;width:100%;text-align:center;border:none;background:transparent;cursor:pointer' value='" +
        location.protocol + "//" +  location.host + "/S/" + aid.substring(1) +
        "' onclick='this.select();document.execCommand(\"copy\");$popup(\"已复制\",\"#088\")' readonly>" +
        "<i class='control icon-link' onclick='this.previousElementSibling.click()'></i>" + 
        "</div></div>");

    divclose.style.maxWidth = $q("#container").style.maxWidth;
    div.CLOSER = divclose.querySelector('.icon-left-small')
    div.CLOSER.onclick = function() {
        div.parentNode.removeChild(div)

        if ($q('[data-parent]', true).length === 0) {
            history.pushState("", "", location.pathname + (window.IS_MEDIA ? '?media=1' : ''));
            document.body.style.overflow = null;
        }
    }

    div.appendChild(divclose);
    document.body.appendChild(div);
    document.body.style.overflow = 'hidden';

    $post('/api/p/' + aid, {}, function(h) {
        div.innerHTML = h;
        div.style.backgroundImage = null;
        var rows = div.querySelector('.rows'),
            box = div.querySelector(".reply-table textarea");

        if (box) window.TRIBUTER.attach(box);
        rows.insertBefore(divclose.querySelector('.row'), rows.firstChild);
    });

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

function updateSetting(el, field, value, cb, errcb) {
    var data = {}, stop = $wait(el.tagName === 'INPUT' ? el.nextElementSibling : el);
    data["set-" + field] = "1";
    if (field !== 'bio') {
        data[field] = value;
    } else {
        ["description"].forEach(function(id) { data[id] = $q("[name='" + id + "']").value })
    }
    $post("/api/user_settings", data, function(h, h2) {
        stop();
        if (cb) cb(h, h2);
        return h
    }, function() {
        stop();
        if (errcb) errcb();
    })
}

function $check(el) {
    var i = el.querySelector('i');
    if (i.className == 'icon-ok-circled2') {
        i.className = 'icon-ok-circled-1';
        el.setAttribute("value", "true")
    } else {
        i.className = 'icon-ok-circled2';
        el.setAttribute("value", "")
    }
}

function showInfoBox(el, uid) {
    if (el.BLOCK) return;
    el.BLOCK = true;

    var div = $q("<div>"),
        loading = $html("<div style='position:absolute;left:0;top:0;width:100%;height:100%'></div>"),
        bodyBox = document.body.getBoundingClientRect(),
        box = el.getBoundingClientRect();

    document.body.appendChild(div);
    div.innerHTML = $q("#dummy-user").innerHTML;
    div.querySelector('img.avatar').src = el.src;

    for (var x = el.parentNode; x ; x = x.parentNode) {
        var pa = x.querySelector('span.post-author')
        if (pa) {
            div.querySelector('span.post-author').innerHTML = pa.innerHTML;
            break;
        }
    }

    for (var x = el.parentNode; x ; x = x.parentNode) {
        if (x.getAttribute('data-id')) {
            box = x.getBoundingClientRect();
            break;
        }
    }

    div.style.position = 'absolute';
    div.style.left = box.left - bodyBox.left + 'px';
    div.style.top = box.top - bodyBox.top + 'px';

    // div.appendChild(loading);
    // loading.style.backgroundImage = 'url(/s/css/spinner.gif)';
    // loading.style.backgroundPosition = 'center';
    // loading.style.backgroundRepeat = 'no-repeat';

    window.REGIONS = window.REGIONS || [];
    window.REGIONS.push({
        valid: true,
        boxes: [div.getBoundingClientRect()],
        callback: function(x, y) {
            div.parentNode.removeChild(div);
            el.BLOCK = false;
        },
    });

    $post("/api/u/" + uid, {}, function(h) {
        if (h.indexOf("ok:") > -1) {
            div.innerHTML = h.substring(3);

            return
        }
        return h
    }, function() {
        el.BLOCK = false;
    })
}

function adjustImage(img) {
    var ratio = img.width / img.height,
        div = img.parentNode.parentNode,
        note = div.querySelector('.long-image');

    if (ratio < 0.33 || ratio > 3) {
        div.style.backgroundSize = 'contain';
        note.style.display = 'block';
    } else {
        div.style.backgroundSize = 'cover';
    }

    if (img.src.match(/mime~gif/)) {
        note.style.display = 'block';
        note.innerText = 'GIF';
    }

    div.style.backgroundImage = 'url(' + img.src + ')';
}
