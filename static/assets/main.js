window.CONST_loaderHTML = "<div class=lds-dual-ring></div>";

function fixCode(inTable) { 
    $('code').each(function(_, c) {
        c = $(c);
        c.text(c.text().replace(/^\n/, ''));
        $("<div style='border-radius:50%;position:absolute;top:0;right:0;background:rgba(255,255,255,0.66)'>").
            append($("<div class='tag-edit-button icon-docs'>").click(function() {
                navigator.clipboard.writeText(c.text());
                // $(this).parent().append($("<span>").text('已复制'));
                const range = document.createRange();
                range.selectNode(c.get(0));
                window.getSelection().removeAllRanges();
                window.getSelection().addRange(range);
            })).insertBefore(c.parent());
    });
}

(function($) {
	$.fn.linedtextarea = function() {
		return this.each(function() {
            const that = $(this);
            that.wrap($('<div style="width:100%">')).on('keydown', function(ev) {
                if (ev.keyCode == 13 || ev.keyCode == 9) {
                    const ta = that.get(0), end = ta.selectionEnd, previous = ta.value.slice(0, end);
                    const lastline = previous.substring(previous.lastIndexOf("\n") + 1).match(/^\s+/);
                    const n = ev.keyCode == 9 ? "\t" : "\n" + (lastline && lastline[0] ? lastline[0] : '');
                    ta.value = previous + n + ta.value.slice(end);
                    ta.selectionStart = end + n.length;
                    ta.selectionEnd = end + n.length;
                    ev.preventDefault();
                }
                previewBtn.hasClass('icon-toggle-on') && previewBtn.click();
            });
            const preview = $('<div style="background:#ffd;padding:0 .5em;overflow-x:hidden;border-bottom:solid 1px #ccc;display:none">');
            const previewBtn = $('<div title="预览" class="icon-toggle-off tag-edit-button">').click(function() {
                if (previewBtn.hasClass('icon-toggle-on')) {
                    previewBtn.toggleClass('icon-toggle-on').toggleClass('icon-toggle-off');
                    preview.html('').hide();
                    return;
                }
                ajaxBtn(previewBtn.get(0), 'preview', {'content': that.val()}, function(data) {
                    previewBtn.toggleClass('icon-toggle-on').toggleClass('icon-toggle-off')
                    preview.css('width', $('.table').width());
                    preview.html(data.content).show();
                    fixCode(true);
                });
            })
            !that.attr('readonly') && that.parent().
                prepend(preview).
                prepend($('<div style="padding:0.25em;display:flex;align-items:center;width:100%;background:rgba(0,0,0,0.05);box-shadow:0 1px 1px rgba(0,0,0,0.2)">').
                    append($('<div title="URL Escape" class="icon-percent tag-edit-button">').click(function(){
                        insert(function(o) {
                            var decoded = o;
                            try { decoded = decodeURIComponent(o) } catch {}
                            return decoded == o ? encodeURIComponent(o) : decoded;
                        });
                    })).
                    append($('<div title="HTML Escape" class="icon-quote-left tag-edit-button">').click(function() {
                        const m = {'&': "&amp;", '<': "&lt;", '>': "&gt;", '"': "&quot;", "'": "&#039;"};
                        insert(function(o) {
                            var decoded = o, encoded = o;
                            Object.keys(m).forEach(function(k) { decoded = decoded.replace(new RegExp(m[k], 'g'), k) });
                            Object.keys(m).forEach(function(k) { encoded = encoded.replace(new RegExp(k, 'g'), m[k]) });
                            return decoded == o ? encoded : decoded;
                        });
                    })).
                    append($('<div title="创建链接" class="icon-link tag-edit-button">').click(function() {
                        insert(function(o) { return "<a href='" + o + "'>" + o + "</a>"; });
                    })).
                    append($('<div title="消空格" class="icon-myspace tag-edit-button">').click(function(){
                        const ta = that.get(0), end = ta.selectionEnd, before = ta.value.slice(0, end), after = ta.value.slice(end);
                        ta.focus();
                        if (after.trimStart().startsWith('>')) {
                            const style = ' style="white-space:normal"';
                            ta.value = before + style + after;
                            ta.selectionStart = end + 1;
                            ta.selectionEnd = end + style.length;
                        } else {
                            ta.value = before + "<eat>" + after;
                            ta.selectionStart = end + 5;
                            ta.selectionEnd = end + 5;
                        }
                    })).
                    append($('<div title="格式化" class="icon-magic tag-edit-button">').click(function() {
                        var result = '', indent = '';
                        that.val().split(/>\s*</).forEach(function(element) {
                            if (element.match(/^\/\w/)) indent = indent.substring(1);
                            result += indent + '<' + element + '>\r\n';
                            if (element.match(/^<?\w[^>]*[^\/]$/) &&
                                !element.match(/^(area|base|br|col|embed|hr|img|input|link|meta|source|track|wbr)/)) {
                                indent += ' ';              
                            }
                        });
                        that.val(result.substring(1, result.length-3));
                    })).
                    append($('<div title="移除HTML标签" class="icon-trash tag-edit-button">').click(function() {
                        const div = document.createElement('div');
                        div.innerHTML = that.val();
                        function walk(node) {
                            var result = '';
                            for (var child = node.firstChild; child; child = child.nextSibling) {
                                if (child.nodeType == Node.TEXT_NODE) {
                                    result += child.data;
                                } else if (child.nodeType == Node.ELEMENT_NODE) {
                                    result += walk(child);
                                }
                            }
                            return result;
                        }
                        that.val(walk(div).replace(/[ \t]+/g, ' ').replace(/^\s+/mg, '').replace(/\n+/g,'\n'));
                    })).
                    append($('<div style="flex-grow:1">')).
                    append(previewBtn)
                );
            function insert(f) {
                const ta = that.get(0), start = ta.selectionStart, end = ta.selectionEnd;
                const res = f(ta.value.slice(start, end));
                ta.focus();
                ta.value = ta.value.slice(0, start) + res + ta.value.slice(end);
                ta.selectionStart = start;
                ta.selectionEnd = start + res.length;
            }
		});
	};

	$.fn.imageSelector = function(onload) {
        const that = this;
        const readonly = !!this.attr('readonly'), defaultImage = this.attr('default'), largeImage = this.attr('large');
        const processing = $("<div class='title'>").hide();
        const viewLarge = $("<div class='title view-large'>").append($("<span class=icon-eye>")).hide();
        const div = $('<div class="image-selector-container">').
            css('cursor', readonly ? 'inherit' : 'pointer').
            attr('readonly', readonly).
            append($("<div>").append($("<img>"))).
            append(processing).
            append(viewLarge).
            click(function() { !readonly && that.click() });
        div.insertBefore(this.hide());

        function onChange(fileIdx, files) {
            function finish(changed, small, display, image, thumb) {
                div.find('img').get(0).src = display;
                that.get(0).imageData = {
                    'image': image,
                    'thumb': thumb,
                    'image_changed': changed,
                    'image_small': small,
                    'image_index': fileIdx,
                    'image_total': files.length,
                    'file_type': image ? (image.type || '') : '',
                    'file_name': image.name,
                    'file_size': image.size,
                };
                processing.hide();
                display ? viewLarge.show().off('click').click(function(ev) {
                    window.open(URL.createObjectURL(image));
                    ev.stopPropagation();
                }) : viewLarge.hide();
                display && onload && onload.apply(that, [that.get(0).imageData, file, fileIdx < files.length - 1 ? function() {
                    onChange(fileIdx + 1, files);
                } : false]);
            }

            if (fileIdx >= files.length) {
                finish(false, false, '', null, null);
                return;
            }

            processing.text('处理中').show();
            const file = files[fileIdx];
            if (!file.type.startsWith("image/")) {
                const canvas = document.createElement("canvas");
                canvas.width = 300; canvas.height = 300;
                const ctx = canvas.getContext("2d");
                ctx.fillStyle = 'white';
                ctx.fillRect(0, 0, 300, 300);

                ctx.fillStyle = 'black';
                ctx.font = "48px serif";
                ctx.textAlign = 'center';
                ctx.textBaseline = 'middle';
                ctx.fillText(file.type.replace(/.+\//, '').toUpperCase() + " " + (file.size / 1024 / 1024).toFixed(2) + 'M', 150, 120);
                ctx.font = '32px serif';
                ctx.fillText(file.name, 150, 160);
                canvas.toBlob(function(blob)  {
                    finish(true, false, canvas.toDataURL('image/jpeg'), file, new File([blob], "thumb.jpg", { type: "image/jpeg" }));
                    $('#title').val(file.name);
                }, 'image/jpeg');
                return;
            }

            const reader = new FileReader(), size = 300;
            reader.onload = function (e) {
                var img = document.createElement("img");
                img.onload = function (event) {
                    if (file.size < 1024 * 100) {
                        finish(true, true, URL.createObjectURL(file), file, null);
                        return;
                    }

                    const canvas = document.createElement("canvas");
                    canvas.width = size; canvas.height = size;
                    const ctx = canvas.getContext("2d");
                    if (img.width > img.height) {
                        var h = size, w = size / img.height * img.width, x = (w - size) / 2, y = 0;
                    } else {
                        var w = size, h = size / img.width * img.height, x = 0, y = (h - size) / 2;
                    }
                    ctx.drawImage(img, -x, -y, w, h);
                    canvas.toBlob(function(blob)  {
                        finish(true, false, canvas.toDataURL('image/jpeg'), file, new File([blob], "thumb.jpg", { type: "image/jpeg" }));
                    }, 'image/jpeg');
                }
                img.onerror = function() {
                    alert('无效图片: ' + file.name);
                    finish(false, false, '', null, null);
                }
                img.src = e.target.result;
            }
            reader.readAsDataURL(file);
        }
        this.change(function() {
            const files = [];
            for (var i = 0; i < that.get(0).files.length; i ++) files.push(that.get(0).files[i]);
            files.sort(function(a, b) { return a.name < b.name ? -1 : 1})
            onChange(0, files);
        });
        this.get(0).imageData = {};
        defaultImage && (div.find('img').get(0).src = defaultImage);
        largeImage && viewLarge.show().click(function(ev) {
            window.open(largeImage);
            ev.stopPropagation();
        })

        if (!readonly) {
            window.lastChangeImage = onChange;
            processing.text('选择或粘贴图片').show();
        }
        return div;
    }
})(jQuery);

function openImage(src) {
    const images = $('.image');
    function move(offset) {
        var idx = 0, current = dialog.find('div.image-container').attr('current');
        images.each(function(i, img) {
            if ($(img).attr('data-src') == current) {
                idx = i;
                return false;
            }
        });
        idx += offset;
        if (idx < 0 || idx >= images.length) {
            dialog.remove();
            document.body.style.overflow = '';
            return;
        }
        dialog.find('div.image-container-info').text((idx+1) + ' / ' + images.length);
        load($(images[idx]).attr('data-src'))
    }
    const dialog = $("<div class=dialog>").css('cursor', 'pointer').
        append($("<div class=image-container>")).
        append($("<div class=image-container-left>")).
        append($("<div class=image-container-right>")).
        append($("<div class=image-container-info>"))
    dialog.on('mouseup', function(ev) {
        if (ev.which != 1) return;
        const x = ev.clientX, mark = dialog.width() / 4;
        if (x <= mark) {
            move(-1);
        } else if (x >= mark * 3) {
            move(1);
        } else {
            dialog.remove();
            document.body.style.overflow = '';
        }
    });
    document.body.appendChild(dialog.get(0));
    document.body.style.overflow = 'hidden';
    function load(src) {
        const con = dialog.find('div.image-container').attr('current', src).html('').append(window.CONST_loaderHTML);
        const img = new Image();
        img.onload = function() {
            const imgc = $("<img>");
            const w = img.width, h = img.height;
            if (w > dialog.width() || h > dialog.height()) {
                var w0 = dialog.width(), h0 = dialog.width() / w * h;
                if (h0 > dialog.height()) {
                    h0 = dialog.height();
                    w0 = dialog.height() / h * w;
                }
                imgc.width(w0).height(h0);
            }
            if (con.attr('current') != src) return;
            con.html('').append(imgc.attr('src', img.src));
        }
        img.src = src;
    }
    dialog.find('div.image-container').attr('current', src)
    move(0);
}

function ajaxBtn(el, action, args, f) {
    if (!el)
        el = document.createElement("div");
    const that = $(el);
    if (that.attr('busy') == 'true') return;
    const fd = new FormData();    
    for (const k in args) fd.append(k, args[k]);
    const rect = el.getBoundingClientRect();
    const loader = $("<div class='ajax-loader' style='display:inline-flex;align-items:center;justify-content:center'>").
        append(window.CONST_loaderHTML).
        css('width', rect.width).
        css('height', rect.height).
        css('margin', $(el).css('margin'));
    !that.prev().hasClass('ajax-loader') && loader.insertBefore(that.attr('busy', 'true').hide());
    function finish() {
        that.attr('busy', '').show();
        loader.remove();
        while (that.prev().hasClass('ajax-loader')) that.prev().remove();
    }
    $.ajax({
        url: '/ns:action',
        data: fd,
        processData: false,
        contentType: false,
        type: 'POST',
        headers: { 'X-Ns-Action': action },
        success: function(data){
            if (!data.success) {
                data.code == "COOLDOWN" ?
                    alert('操作频繁，请在 ' + data.remains + ' 秒后重试') : alert(data.msg + ' (' + data.code + ')');
                finish();
                return;
            }
            that.attr('busy', '');
            if (f) {
                (f(data, args) !== "keep_loading") && finish();
                return;
            }
            finish();
            location.reload();
        },
        error: function() {
            alert('网络错误，请稍后重试');
            finish();
        },
    });
}

$(document).ready(function() {
    $(".image-selector").each(function(_, i) {
        $(i).imageSelector();
    });

    fixCode();
})

document.onpaste = function (event) {
    var items = (event.clipboardData || event.originalEvent.clipboardData).items;
    for (var index in items) {
        var item = items[index];
        if (item.kind === 'file' && window.lastChangeImage) {
            window.lastChangeImage(0, [item.getAsFile()]);
            break;
        }
    }
};
