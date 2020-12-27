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

    var ids = [];
    $q('#' + cid + ' .dz-remove', true).forEach(function(el) {
        var u = el.getAttribute("data-uri"); u ? ids.push(u) : 0;
    })

    var stop = $wait(el);
    $post("/api2/new", {
        content: ta.value,
        media: ids.join(';'),
        nsfw: $q("#" + cid + " [name=isnsfw]").checked ? "1" : "",
        anon: $q("#" + cid + " [name=anon]").value,
        no_master: $q("#" + cid + " [name=nomaster]").checked ? "1" : "",
        no_timeline: $q("#" + cid + " [name=notimeline]").checked ? "1" : "",
        stick_on_top: $q("#" + cid + " [name=stickontop]").checked ? "1" : "",
        poll: $q("#" + cid + " [name=poll]").checked ? "1" : "",
        reply_lock: $value($q("#" + cid + " [name=reply-lock]")),
        parent: p,
    }, function (res, h) {
        stop();
        if (res.substring(0, 3) !== "ok:") {
            return res;
        } else {
            ta.value = "";
            $q('#' + cid + ' .dropzone').UPLOADER.removeAllFiles();
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
        add = function(i, front, ac) {
            var li = $q("<li>"), img = $q("<img>"), idx = ac ? i : ("000"+i).slice(-3);
            img.src = ac ?
		       "/s/emoji/" + idx + '.png' :
		       'https://static.saraba1st.com/image/smiley/face2017/' + idx + '.png';
            img.setAttribute("loading", "lazy");
            img.setAttribute("class", ac ? "ac-emoji" : "");
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
            var i = (e || {}).k || "";
            if (!i.match(/(\d{3}|ac\d+|a2_\d+)/)) return;
            omits.push(i);
            add(i, true, i.substr(0, 1) == 'a');
        });

    for (var i = 1; i <= 226; i++) {
        if (omits.includes(i)) continue;
        add(i);
    }

    var ac = ["ac0", "ac1", "ac2", "ac3", "ac4", "ac5", "ac6", "ac8", "ac9", "ac10", "ac11", "ac12", "ac13", "ac14", "ac15", "ac17", "ac23", "ac21", "ac33", "ac34", "ac35", "ac36", "ac37", "ac22", "ac24", "ac25", "ac26", "ac27", "ac28", "ac29", "ac16", "ac18", "ac19", "ac20", "ac30", "ac32", "ac40", "ac44", "ac38", "ac43", "ac31", "ac39", "ac41", "ac7", "ac42", "a2_02", "a2_05", "a2_03", "a2_04", "a2_07", "a2_08", "a2_09", "a2_10", "a2_14", "a2_16", "a2_15", "a2_17", "a2_21", "a2_23", "a2_24", "a2_25", "a2_27", "a2_28", "a2_30", "a2_31", "a2_32", "a2_33", "a2_36", "a2_51", "a2_53", "a2_54", "a2_55", "a2_47", "a2_48", "a2_45", "a2_49", "a2_18", "a2_19", "a2_52", "a2_26", "a2_11", "a2_12", "a2_13", "a2_20", "a2_22", "a2_42", "a2_37", "a2_38", "a2_39", "a2_41", "a2_40"];
    for (var i = 0; i < ac.length; i++) {
        if (omits.includes(i)) continue;
	add(ac[i], false, true);
    }
}
