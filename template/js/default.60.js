(function() {
    window.onmousemove = function(e) {
        if (window.REGTICK) return;
        window.requestAnimationFrame(function() {
            var x = e.clientX || e.left, y = e.clientY || e.top;
            window.REGIONS = (window.REGIONS || []).filter(function(rect) { return rect.valid; })
            window.REGIONS.forEach(function(rect) {
                var inside = false, margin = 5;
                rect.boxes.forEach(function(el) {
                    var box = el.getBoundingClientRect();
                    inside = inside || (x >= box.left - margin && x <= box.right + margin &&
                        y >= box.top - margin && y <= box.bottom + margin);
                })
                if (!inside) {
                    try {
                        rect.callback(x, y);
                    } catch(e) {
                        console.log(e)
                    }
                    rect.valid = false;
                }
            })
            window.REGTICK = false;
        });
        window.REGTICK = true;
    }

    window.ontouchend = function(e) {
        var el = e.changedTouches[0];
        if (el) window.onmousemove(el);
    }

    window.addEventListener('scroll', function(e) {
        var nextBtn = $q("#load-more")
        if (!nextBtn) return;
        if (window.ticking) return
        window.requestAnimationFrame(function() {
            $q(".timeline > .article-row", true).forEach(function(c) {
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
    });
})()

function $q(q, multi) {
    if (q.match(/^<\S+>$/)) return document.createElement(q.substring(1, q.length - 1));
    var el = document.querySelectorAll(q), ela = [];
    if (el.length === 1 && !multi) return el[0];
    for (var i = 0; i < el.length; i++) ela.push(el[i]);
    return ela;
}

function isString(a) {
    return (Object.prototype.toString.call(a) === '[object String]');
}

function $html(html) {
    var div = document.createElement("div")
    div.innerHTML = html;
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
    var div = $html("<div class=toast></div>");
    div.innerHTML = html;
    div.onclick = function() {
        div.style.transition = "opacity 0.8s";
        div.style.opacity = "0";
        setTimeout(function() {div.parentNode.removeChild(div)}, 1000);
    }
    document.body.appendChild(div);
    setTimeout(div.onclick, 2000)
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
            $popup('<i class=icon-ok-circled></i>成功');
        } else if (cbres && cbres.match(/^ok:/)) {
            $popup('<i class=icon-ok-circled></i>' + cbres.substring(3));
        } else {
            if (errorcb) errorcb(xml)
            var text = "错误状态: " + xml.status, reason = xml.getResponseHeader("X-Reason");
            if (xml.status === 404) text = "内容未找到";
            if (reason) text = __i18n(reason);
            $popup('<i class=icon-cancel-circled-1></i>' + (cbres || text));
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
        try {
            var r = new URLSearchParams(window.location.search).get('redirect');
            if (r) {
                location.href = r;
                return;
            }
        } catch(e) {}
        location.reload();
    }, stop)
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
            icon.className = 'icon-heart-2';
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
        $q("[data-pre-id='" + id + "']", true).forEach(function(e) {
            e.innerHTML = "<span class=deleted></span>";
        });
        $q("[data-media-id='" + id + "'] img", true).forEach(function(e) {
            e.src = '';
        });
        $q("[data-media-id='" + id + "']", true).forEach(function(e) {
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
        return "ok";
    }, stop);
}

function dropTopArticle(el, id) {
    if (!confirm("是否取消置顶")) return;
    $postReload(el, "/api2/drop_top", { id: id, extra: "" })
}

function lockArticle(el, id) {
    var div = $html("<div class=tmpl-light-bg style='border-radius:0.5em;position:absolute;z-index:1001;box-shadow:0 1px 5px rgba(0,0,0,.3)'></div>"),
        box = el.getBoundingClientRect(),
        bodyBox = document.body.getBoundingClientRect(),
        currentValue = $value(el),
        reg = {};

    div.style.left = box.left - bodyBox.left + "px";
    div.style.top = box.bottom - bodyBox.top + "px";

    var checkbox = function(i, t) {
        var tmpl = "<div style='margin:0.5em'><input id=ID CHECKED type=radio value=V name=reply-lock> <label for=ID>TEXT</label></div>"
        return $html(tmpl.replace("V",i).replaceAll("ID","id"+Math.random()).replace("CHECKED",i==currentValue?"checked=checked":"").replace("TEXT",t))
    }

    div.appendChild(checkbox(0, "不限制回复"))
    div.appendChild(checkbox(1, "禁止回复"))
    div.appendChild(checkbox(2, "我关注的人可回复"))
    div.appendChild(checkbox(3, "我关注的人和被@的人可回复"))
    div.appendChild(checkbox(4, "我关注的人和我粉丝可回复"))
    document.body.appendChild(div)

    if (id)
        div.appendChild($html("<div style='margin:0.5em;text-align:center'><button class=gbutton>更新设置</button></div>"))

    reg = {
        valid: true,
        boxes: [el, div],
        callback: function(x, y) {
            if (!id) {
                var v = (div.querySelector("[name=reply-lock]:checked") || {}).value;
                if (v) el.setAttribute("value", v)
            }
            div.parentNode.removeChild(div);
        },
    };
    window.REGIONS = window.REGIONS || [];
    window.REGIONS.push(reg);

    if (!id) return;
    div.querySelector('button').onclick = function(e) {
        var stop = $wait(e.target), v = div.querySelector("[name=reply-lock]:checked").value;
        $post("/api2/toggle_lock", { id: id, mode: v }, function (res) {
            stop();
            if (res != "ok") return res;
            el.setAttribute("value", v)
            el.innerHTML = "<i class='C'></i>".replace("C", v > 0 ? "tmpl-normal-text icon-lock" : "tmpl-light-text icon-lock-open")
            return "ok:回复设置更新"
        }, stop);
    }
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
    $post("/api2/follow_block", obj, function(res, x) {
        stop();
        if (res != "ok") return res;

	var on = obj[m] != "";
	el.setAttribute("value", on ? "true" : "false");
        if (m == "follow") {
            el.innerHTML = "<i class=C></i>".replace("C", on ? "icon-heart-broken" : "icon-user-plus");
            if (x.getResponseHeader("X-Follow-Apply") && on)
                return "ok:已关注, 等待批准";
            return "ok:" + (on ? "已关注" : "已取消关注") + id;
        } else if (m == "accept") {
            el.innerHTML = '<i class="icon-ok tmpl-green-text"></i>';
            return "ok" 
        } else {
            el = el.querySelector('i');
            el.className = el.className.replace(/block-\S+/, '') + " block-" + on;
            el = el.nextElementSibling;
            if (el) el.innerText = on ? "解除" : "拉黑";
            return "ok:" + (on ? "已拉黑" + id : "已解除" + id + "拉黑状态")
        }
    }, stop)
}

function __i18n(t) {
    if (t.match(/cooldown`([0-9\.]+)s/)) 
        return "请等待" + t.split("`").pop();
    if (t === "captcha_failed")
        return "无效验证码";
    if (t === "expired_session")
        return "Token过期，请重试";
    if (t === "content_too_short")
        return "正文过短";
    if (t === "cannot_reply")
        return "无法回复";
    if (t === "internal_error")
        return "服务端异常";
    if (t === "user_not_found")
        return "无权限";
    if (t === "user_not_found_by_id")
        return "ID不存在";
    if (t === "new_password_too_short")
        return "新密码太短";
    if (t === "old_password_invalid")
        return "旧密码不符";
    if (t === "duplicated_id")
        return "ID已存在";
    if (t === "id_too_short")
        return "无效ID";
    if (t === "invalid_id_password")
        return "无效ID或密码";
    if (t === "user_not_permitted")
        return "无权限";
    if (t === "cannot_follow")
        return "无法关注";
    if (t === "cannot_block_tag")
        return "无法拉黑标签";
    return t;
}

function loadMore(el, data) {
    data.cursors = $value(el);
    var stop = $wait(el);
    $post('/api/timeline', data, function(pl) {
        stop();
        if (pl.EOT) {
            el.innerText = "没有更多内容了";
            el.setAttribute("eot", "true");
            el.className += " tmpl-light-text";
            el.onclick = function() { location.reload() }
        } else {
            el.innerText = "更多...";
        }
        if (pl.Articles) {
            el.setAttribute("value", pl.Next);
            pl.Articles.forEach(function(a) {
                var dedup = $q(".timeline > [data-id='" + a[0] + "']");
                if (dedup && dedup.length) {
                    console.log("dedup:", a[0])
                    return;
                }
                $q('.timeline').appendChild($html(a[1]));
            })
        }
    }, stop);
}

function updateSetting(el, field, value) {
    var data = {};
    var stop = $wait(el.tagName === 'INPUT' && el.getAttribute('type') == "checkbox" ?  el.nextElementSibling: el);
    data["set-" + field] = "1";
    data[field] = value;
    $post("/api/user_settings", data, function(h, h2) {
        stop();
        return h
    }, stop)
}

function showInfoBox(el, uid) {
    if (uid.substr(0,1) == "?") return;
    if (el.BLOCK) return;
    el.BLOCK = true;

    var div = $q("<div>"),
        loading = $html("<div style='position:absolute;left:0;top:0;width:100%;height:100%'></div>"),
        bodyBox = document.body.getBoundingClientRect(),
        box = el.getBoundingClientRect(),
        boxTopOffset = 0,
        addtionalBoxes = [],
        startAt = new Date().getTime();

    div.className = 'user-info-box';
    div.innerHTML = window.DUMMY_USER_HTML;
    div.querySelector('img.avatar').src = el.src || '';
    div.querySelector('img.avatar').onclick = null;

    if (el.className === 'mentioned-user') {
        div.querySelector('span.post-author').innerHTML = el.innerHTML;
    } else {
        for (var x = el.parentNode; x ; x = x.parentNode) {
            var pa = x.querySelector('span.post-author')
            if (pa) {
                div.querySelector('span.post-author').innerHTML = pa.innerHTML;
                break;
            }
        }
    }

    for (var x = el; x ; x = x.parentNode) {
        if (x.getAttribute('data-id') || x.className === 'mentioned-user') {
            box = x.getBoundingClientRect();
            if (x.className === 'mentioned-user') {
                addtionalBoxes.push(x);
                boxTopOffset = box.bottom - box.top;
            }
            break;
        }
    }

    div.style.position = 'absolute';
    div.style.left = box.left - bodyBox.left + 'px';
    div.style.top = box.top - bodyBox.top + boxTopOffset + 'px';
    div.style.boxShadow = "0 1px 2px rgba(0, 0, 0, .3), 0 0 2px rgba(0,0,0,.2)";
    document.body.appendChild(div);

    var reg = {
        valid: true,
        boxes: [div].concat(addtionalBoxes),
        callback: function(x, y) {
            div.parentNode.removeChild(div);
            el.BLOCK = false;
        },
    };

    window.REGIONS = window.REGIONS || [];
    window.REGIONS.push(reg);

    var adjustDiv = function() {
        var newBox = div.getBoundingClientRect();
        if (newBox.right > bodyBox.right) {
            div.style.left = '0';
            div.style.right = "0";
        }
        div.querySelector('.article-row').className += ' tmpl-light-bg'
    }
    $post("/api/u/" + encodeURIComponent(uid), {}, function(h) {
        if (h.indexOf("ok:") > -1) {
            setTimeout(function() {
                div.innerHTML = h.substring(3)
                adjustDiv();
            }, new Date().getTime() - startAt > 100 ? 0 : 100)
            return
        }
        return h
    }, function() {
        adjustDiv();
        el.BLOCK = false;
    })
}

function adjustImage(img) {
    var ratio = img.width / img.height,
        div = img.parentNode.parentNode,
        note = div.querySelector('.long-image'),
        r = div.getBoundingClientRect(),
        smallimg = false,
        container = div.parentNode;

    while (!container.className.match(/media-container/)) container = container.parentNode;

    if (ratio < 0.33 || ratio > 3) {
        div.style.backgroundSize = 'contain';
        note.style.display = 'block';
    } else {
        div.style.backgroundSize = 'cover';
    }

    if (img.width <= r.width * 0.9 && img.height <= r.height * 0.9) {
        div.style.backgroundSize = 'auto';
        smallimg = true;
    }

    if (img.src.match(/\.gif$/)) {
        note.style.display = 'block';
        note.innerText = 'GIF';
    }

    if (div.hasAttribute("enlarge")) {
        div.style.height = window.innerHeight + "px";
        div.scrollIntoView();
        div.style.backgroundSize = 'contain';
    }

    div.style.backgroundImage = 'url(' + img.src + ')';
    div.onclick = function() {
        if (!div.hasAttribute("enlarge")) {
            div.setAttribute("enlarge", "enlarge")
            div.style.width = "100%";
            div.style.height = window.innerHeight + "px";
            div.style.borderRadius = '0';
            div.scrollIntoView();
            container.style.marginLeft = '-3.5em';

            var imgload = new Image(), imgprogress = new Image(), divC = $q("<div>"), loaded = false;

            imgload.src = img.src.replace(/\/thumb\//, '/');
            imgload.onload = function() {
                loaded = true;
                img.src = imgload.src; // trigger adjustImage() again
                try { div.removeChild(divC) } catch (e) {}
            }

            imgprogress.src =  '/s/assets/spinner.gif';
            imgprogress.setAttribute('style', 'opacity:unset;display:block;position:absolute;top:50%;left:50%;transform:translate(-50%,-50%);');
            divC.className = 'image-loading-div';
            divC.appendChild(imgprogress);
            div.appendChild(divC);
            setTimeout(function() { if (!loaded) divC.style.opacity = '1' }, 100)
        } else {
            div.removeAttribute("enlarge")
            div.style.borderRadius = null;
            div.style.width = null;
            div.style.height = null;
            div.parentNode.querySelector('[image-index="0"]').scrollIntoView();
            container.style.marginLeft = '0';
        }
    }
}

function adjustVideoIFrame(el, src) {
    el.style.display = 'none';
    el.previousElementSibling.style.display = 'none';
    el = el.nextSibling;
    el.style.display = null;
    var box = el.getBoundingClientRect();
    var w = box.right - box.left;
    el.style.height = (el.getAttribute("fixed-height") || (w*0.75)) + 'px';
    el.src = src;
}

function isDarkMode() {
    // return (document.cookie.match(/(^| )mode=([^;]+)/) || [])[2] === 'dark';
}

function createAvatar(id) {
    var randseed = new Array(4); // Xorshift: [x, y, z, w] 32 bit values

    function seedrand(seed) {
        randseed.fill(0);

        for(let i = 0; i < seed.length; i++) {
            randseed[i%4] = ((randseed[i%4] << 5) - randseed[i%4]) + seed.charCodeAt(i);
        }
    }

    function rand() {
        // based on Java's String.hashCode(), expanded to 4 32bit values
        const t = randseed[0] ^ (randseed[0] << 11);

        randseed[0] = randseed[1];
        randseed[1] = randseed[2];
        randseed[2] = randseed[3];
        randseed[3] = (randseed[3] ^ (randseed[3] >> 19) ^ t ^ (t >> 8));

        return (randseed[3] >>> 0) / ((1 << 31) >>> 0);
    }

    function createColor() {
        //saturation is the whole color spectrum
        const h = Math.floor(rand() * 360);
        //saturation goes from 40 to 100, it avoids greyish colors
        const s = ((rand() * 40) + 30) + '%';
        //lightness can be anything from 0 to 100, but probabilities are a bell curve around 50%
        const l = ((rand() + rand() + rand() + rand()) * 25) + '%';

        return 'hsl(' + h + ',' + s + ',' + l + ')';
    }

    function createImageData(size) {
        const width = size; // Only support square icons for now
        const height = size;

        const dataWidth = Math.ceil(width / 2);
        const mirrorWidth = width - dataWidth;

        const data = [];
        for(let y = 0; y < height; y++) {
            let row = [];
            for(let x = 0; x < dataWidth; x++) {
                // this makes foreground and background color to have a 43% (1/2.3) probability
                // spot color has 13% chance
                row[x] = Math.floor(rand()*2.3);
            }
            const r = row.slice(0, mirrorWidth);
            r.reverse();
            row = row.concat(r);

            for(let i = 0; i < row.length; i++) {
                data.push(row[i]);
            }
        }

        return data;
    }

    function buildOpts(opts) {
        const newOpts = {};

        newOpts.seed = opts.seed || Math.floor((Math.random()*Math.pow(10,16))).toString(16);
        newOpts.seed = newOpts.seed.split("").reduce(function(a,b){a=((a<<5)-a)+b.charCodeAt(0);return a&a},0).toString();

        seedrand(newOpts.seed);

        newOpts.size = opts.size || 8;
        newOpts.scale = opts.scale || 4;
        newOpts.color = opts.color || createColor();
        newOpts.bgcolor = opts.bgcolor || createColor();
        newOpts.spotcolor = opts.spotcolor || createColor();

        return newOpts;
    }

    function renderIcon(opts, canvas) {
        opts = buildOpts(opts || {});
        const imageData = createImageData(opts.size);
        const width = Math.sqrt(imageData.length);

        canvas.width = canvas.height = opts.size * opts.scale;

        const cc = canvas.getContext('2d');
        cc.fillStyle = opts.bgcolor;
        cc.fillRect(0, 0, canvas.width, canvas.height);
        cc.fillStyle = opts.color;

        for(let i = 0; i < imageData.length; i++) {

            // if data is 0, leave the background
            if(imageData[i]) {
                const row = Math.floor(i / width);
                const col = i % width;

                // if data is 2, choose spot color, if 1 choose foreground
                cc.fillStyle = (imageData[i] == 1) ? opts.color : opts.spotcolor;

                cc.fillRect(col * opts.scale, row * opts.scale, opts.scale, opts.scale);
            }
        }

        return canvas;
    }


    var canvas = document.createElement('canvas');
    renderIcon({bgcolor: "#fafbfc", seed: id, size: 5, scale: 10}, canvas);
    return canvas;
}

function onPostFinished(res) {
    var tl = document.getElementById("timeline" + res.uuid);
    if (!tl) return;
    var div = $html(res.html);
    div.className += " newly-added-row"
    setTimeout(function() {
        div.className += " finished"
        setTimeout(function() { div.className = div.className.replace('newly-added-row', '') }, 2000)
    }, 1000)
    tl.insertBefore(div, tl.querySelector(".row-reply-inserter").nextSibling)
    $popup(res.parent ? "回复成功": "发布成功")
}

function postBox(uuid, p, win) {
    function remoteSearch(text, cb) {
        $post("/api/search", { id: text }, function(results) {
            if (results && results.length) {
                results.forEach(function(t, i) {
                    results[i] = { key: t.Display, id: t.ID, is_tag: t.IsTag } 
                });
            }
            JSON.parse(window.localStorage.getItem('presets') || '[]')
                .filter(function(t){ return t; })
                .forEach(function(t) {
                    results.push({ key: t, id: t.substring(1), is_tag: t.substring(0, 1) == '#' }) 
                });
            var seen = {};
            results = results.filter(function(item) {
                return seen.hasOwnProperty(item.key) ? false : (seen[item.key] = true);
            });
            cb(results);
        })
    }

    new Tribute({
        collection: [
            {
                trigger: '@',
                selectClass: 'highlight',
                containerClass: 'tribute-container',
                selectTemplate: function (item) { return '@' + item.original.id; },
                lookup: 'key',
                values: remoteSearch,
            }, {
                trigger: '#',
                selectClass: 'highlight',
                containerClass: 'tribute-container',
                selectTemplate: function (item) { return '#' + item.original.id; },
                lookup: 'key',
                values: remoteSearch,
            }
        ]
    }).attach($q("#post-box textarea"));

    new Dropzone($q("#post-box .dropzone"), {
        url: "/api/upload_image",
        maxFilesize: 16,
        maxFilesize: 5,
        addRemoveLinks: true,
        dictRemoveFile: "<u style='cursor:pointer'>删除</u>",
        dictFileTooBig: "文件过大 (上限5M)",
        dictCancelUpload: "<span class='tmpl-mid-text'>上传中</span> <u style='cursor:pointer'>取消</u>",
        dictCancelUploadConfirmation: "取消上传该图片？"
    }).on("success", function(f, id) {
        f._removeLink.setAttribute('data-uri', id);
    });

    $q("#post-box textarea").focus()

    var box = $q("#post-box");
    box.OLD = box.innerHTML;
    // box.style.display = 'flex';
    box.className += " open";
    document.body.style.overflow = 'hidden';
    history.pushState({}, "发布", "/post_box?p=" + (p||""))
    window.onpopstate = function(event) {
        closePostBox();
    };
}

function closePostBox(historyBack) {
    var el = $q("#post-box");
    el.innerHTML = el.OLD; // clear inside content
    el.className = '';
    document.body.style.overflow = null;
}

function closeWin() {
    var back = window.BACK;
    if (back) {
        var w = window.open('', back)
        if (w.location.href === 'about:blank') {
            w.location.href = '/t?back=' + back
        }
        window.close()
        return;
    }
    location.href = '/t'
}
