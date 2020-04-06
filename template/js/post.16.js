function onPost(uuid, el, p) {
    var cid = "rv-" + uuid,
        ta = $q("#" + cid + " [name=content]"),
        image64 = $q("#" + cid + " [name=image64]"),
        image = $q("#" + cid + " [type=file]"),
        res = ta.value.match(/((@|#)\S+)/g);

    if (res) {
        var e = JSON.parse(window.localStorage.getItem("presets") || "[]");
        e = e.concat(res);
        e = e.filter(function(el, i) { return e.indexOf(el) == i; });
        if (e.length > 16) e = e.slice(e.length - 16);
        window.localStorage.setItem("presets", JSON.stringify(e));
    }

    var stop = $wait(el);
    $post("/api2/new", {
        content: ta.value,
        image64: image64.value,
        image_name: image64.IMAGE_NAME || "",
        nsfw: $q("#" + cid + " [name=isnsfw]").checked ? "1" : "",
        no_master: $q("#" + cid + " [name=nomaster]").checked ? "1" : "",
        no_timeline: $q("#" + cid + " [name=notimeline]").checked ? "1" : "",
        stick_on_top: $q("#" + cid + " [name=stickontop]").checked ? "1" : "",
        reply_lock: $value($q("#" + cid + " [name=reply-lock]")),
        parent: p,
    }, function (res, h) {
        stop();
        if (res.substring(0, 3) !== "ok:") {
            return res;
        } else {
            ta.value = "";
            image64.value = "";
            image.value = null;
            image.onchange();
        }
        var div = $q("<div>")
        div.innerHTML = decodeURIComponent(res.substring(3));
        var tl = $q("#timeline" + uuid);
        tl.insertBefore(div.querySelector("div"), tl.querySelector(".row-reply-inserter").nextSibling)
    }, stop)
}

function onImageChanged(el) {
    var btn = el.previousElementSibling, imageSize = el.parentNode.parentNode.querySelector('.image-size');

    btn.className = btn.className.replace(/image/, "")
    btn.querySelector('div') ? btn.removeChild(btn.querySelector('div')) : 0;
    el.nextElementSibling.value = "";
    imageSize.innerText = "";

    if (!el.value) return;

    var reader = new FileReader();
    reader.readAsDataURL(el.files[0]);
    reader.onload = function () {
        var img = new Image();
        img.onerror = function() {
        }
        img.onload = function() {
            img.onload = null;
            var canvas = document.createElement("canvas"), throt = 1.4 * 1000 * 1000, f = 1,
                success = function() {
                    el.nextElementSibling.value = img.src;
                    el.nextElementSibling.IMAGE_NAME = el.value.split(/(\\|\/)/g).pop();

                    imageSize.innerText = "(" + (img.src.length / 1.33 / 1024).toFixed(0) + "KB)";
                    console.log((f * 100).toFixed(0) + "%)");

                    var img2 = $q("<img>"), div = $q("<div>");
                    img2.src = img.src;
                    div.appendChild(img2);
                    btn.appendChild(div);
                    btn.className += " image";
                };


            // $q('#reply-submit').setAttribute("disabled", "disabled");
            if (img.src.length > throt) {
                if (img.src.match(/image\/gif/)) {
                    img.onerror();
                    return;
                }
                var ctx = canvas.getContext('2d');
                canvas.width = img.width; canvas.height = img.height;
                ctx.drawImage(img,0,0);
                for (f = 0.8; f > 0; f -= 0.2) {
                    var res = canvas.toDataURL("image/jpeg", f);
                    if (res.length <= throt) {
                        img.src = res;
                        success();
                        return;
                    }
                }
                img.onerror();
            } else {
                success();
            }
        }
        img.src = reader.result;
    };
}

function insertMention(btn, id, e) {
    var el = typeof id === 'string' ? $q("#rv-" + id + " [name=content]") : id;
    if (e.startsWith("@") || e.startsWith("#"))
        el.value = el.value.replace(/(@|#)\S+$/, "") + e + " ";
    else
        el.value += e;
    el.focus();
    el.selectionStart = el.value.length;
    el.selectionEnd = el.selectionStart; 
    if (btn) hackHide(btn);
}

function insertTag(btn, id, start, text, end) {
    if (text.length > 300) {
        $popup("文本过长 (300字符)")
        return false;
    }
    var el = typeof id === 'string' ? $q("#rv-" + id + " [name=content]") : id;
    if (el.value) start = "\n" + start;
    el.value += start + text + end;
    el.focus();
    el.selectionStart = el.value.length - end.length - text.length;
    el.selectionEnd = el.selectionStart + text.length;
    hackHide(btn);
}

function hackHide(el) {
    if (!el) return;
    while(el && el.tagName !== 'UL') el = el.parentNode;
    // el.parentNode.onmouseout = function(e) { el.style.display = null; }
    el.style.display='none';
    el.parentNode.onmouseenter = function(e) { el.style.display = null; }
    el.parentNode.onmousemove = function(e) { el.style.display = null; }
}

function emojiMajiang(uuid) {
    var ul = $q('#rv-' + uuid + ' .post-options-emoji ul');
    if (ul.LOADED) return;
    ul.LOADED = true;

    var omits = [4, 9, 56, 61, 62, 87, 115, 120, 137, 168, 169, 211, 215, 175, 210, 213, 209, 214, 217, 206],
        history = JSON.parse(localStorage.getItem("EMOJIS") || '{}'),
        add = function(i, front) {
            var li = $q("<li>"), img = $q("<img>"), idx = ("000"+i).slice(-3);
            img.src = 'https://static.saraba1st.com/image/smiley/face2017/' + idx + '.png';
            img.setAttribute("loading", "lazy");
            img.onclick = function() {
                var e = JSON.parse(localStorage.getItem("EMOJIS") || '{}');
                e[idx] = {w:new Date().getTime(),k:idx};
                localStorage.setItem('EMOJIS', JSON.stringify(e));
                insertMention(img, uuid, '[mj]' + idx + '[/mj]');
            }
            li.appendChild(img);
            front ? ul.insertBefore(li, ul.querySelector('li')) : ul.appendChild(li);
        };

    Object.values(history)
        .sort(function(a,b) { return a.w > b.w })
        .map(function(k) { return history[(k || {}).k] })
        .forEach(function(e) {
            var i = parseInt((e || {}).k, 10);
            if (!i) return;
            omits.push(i);
            add(i, true);
        });

    for (var i = 1; i <= 226; i++) {
        if (omits.includes(i)) continue;
        add(i);
    }
}
