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
        nsfw: $value($q("#" + cid + " [name=isnsfw]")) == 'true' ? "1" : "",
        no_master: $value($q("#" + cid + " [name=nomaster]")) == 'true' ? "1" : "",
        no_timeline: $value($q("#" + cid + " [name=notimeline]")) == 'true' ? "1" : "",
        parent: p,
    }, function (res, h) {
        stop();
        if (res != "ok") {
            return res;
        } else {
            ta.value = "";
            image64.value = "";
            image.value = null;
            image.onchange();
        }
        var div = $q("<div>")
        div.innerHTML = decodeURIComponent(h.getResponseHeader("X-Result"));
        $q("#timeline" + uuid).insertBefore(div.querySelector("div"), $q("#" + cid).nextSibling)
    }, stop)
}

function onContentObserved(el) {
    var emoji = el.parentNode.parentNode.parentNode.querySelector('.emoji');
    var emojilist = emoji.querySelector('ul');

    if (emoji.getAttribute("preloaded") !== "true") {
        try {
            var f = emojilist.querySelector('li');
            JSON.parse(window.localStorage.getItem('presets') || '[]')
                .filter(function(t){return t})
                .forEach(function(t) {
                    var li = $q('<li>'), a =$q('<a>');
                    li.appendChild(a);
                    a.onclick = function() { insertMention(el, t); }
                    a.innerText = t;
                    emojilist.insertBefore(li, f);
                });
        } catch (e) {
            console.log(window.localStorage.getItem('presets'), e);
            window.localStorage.setItem('presets', '');
        }
        emoji.setAttribute("preloaded", "true")
    }

    emoji.style.display = null; 
    var autos = emojilist.querySelectorAll("li.auto");
    for (var i = 0; i < autos.length; i++) emojilist.removeChild(autos[i]);

    var res = el.value.match(/(@\S+|#\S+)$/g);
    if (!res || res.length !== 1 || window.REQUESTING) return;
    window.REQUESTING = true;

    $post("/api/search", {id:res[0].substring(1)}, function(results) {
        if (results && results.length) {
            var f = emojilist.querySelector('li');
            results.forEach(function(t) {
                var li = document.createElement('li');
                var a = document.createElement('a');
                li.appendChild(a);
                li.className = "auto";
                a.onclick = function() { insertMention(el, t); }
                a.innerText = t;
                emojilist.insertBefore(li, f);
            });
        }
        window.REQUESTING = false;
    }, function() {
        window.REQUESTING = false;
    })
}

function onImageChanged(el) {
    var btn = el.previousElementSibling;
    btn.className = btn.className.replace(/image/, "")
    btn.querySelector('div') ? btn.removeChild(btn.querySelector('div')) : 0;
    el.nextElementSibling.value = "";

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
                    // $q('#reply-submit').removeAttribute("disabled");
                    var img2 = document.createElement("img");
                    var div = document.createElement("div");
                    var span = document.createElement("div");
                    img2.src = img.src;
                    div.style.position = 'relative';
                    span.className = 'info';
                    span.innerText += (img.src.length / 1.33 / 1024).toFixed(0) + "KB";
                    span.innerText += (f == 1) ? "" : "/" + (f).toFixed(1);
                    div.appendChild(img2);
                    div.appendChild(span);
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

function insertMention(id, e) {
    var el = typeof id === 'string' ? $q("#rv-" + id + " [name=content]") : id;
    if (e.startsWith("@") || e.startsWith("#"))
        el.value = el.value.replace(/(@|#)\S+$/, "") + e + " ";
    else
        el.value += e;
    el.focus();
}
