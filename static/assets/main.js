window.CONST_closeSVG = "<svg class='svg16 closer' viewBox='0 0 16 16'><circle cx=8 cy=8 r=8 fill='#aaa' /><path d='M 5 5 L 11 11' stroke=white stroke-width=3 fill=transparent /><path d='M 11 5 L 5 11' stroke=white stroke-width=3 fill=transparent /></svg>";
window.CONST_tickSVG = "<svg class='svg16' viewBox='0 0 16 16'><circle cx=8 cy=8 r=8 fill='green' /><path fill=white stroke=transparent d='M 4.139 6.749 L 2.235 8.848 L 6.491 13.156 L 13.534 6.429 L 11.657 4.382 L 6.781 9.244 L 4.139 6.749 Z' /></svg>";
window.CONST_starSVG = "<svg class='svg16 starer' viewBox='0 0 16 16'><circle cx=8 cy=8 r=8 fill='#aaa' /><path d='M 8.065 2.75 L 9.418 6.642 L 13.537 6.726 L 10.254 9.215 L 11.447 13.159 L 8.065 10.806 L 4.683 13.159 L 5.876 9.215 L 2.593 6.726 L 6.712 6.642 Z' fill=white /></svg>";
window.CONST_loaderHTML = "<div class=lds-dual-ring></div>";

/**
 * Adapted from jQuery Lined Textarea Plugin
 * http://alan.blog-city.com/jquerylinedtextarea.htm
 *
 * Released under the MIT License:
 * http://www.opensource.org/licenses/mit-license.php
 */
(function($) {
    $.fn.isInViewport = function() {
        var elementTop = $(this).offset().top;
        var elementBottom = elementTop + $(this).outerHeight();

        var viewportTop = $(window).scrollTop();
        var viewportBottom = viewportTop + $(window).height();

        return elementBottom > viewportTop && elementTop < viewportBottom;
    };
	$.fn.linedtextarea = function() {
		/*
		 * Helper function to make sure the line numbers are always kept up to
		 * the current system
		 */
		var fillOutLines = function(linesDiv, h, lineNo) {
			while (linesDiv.height() < h) {
				linesDiv.append("<div>" + lineNo + "</div>");
				lineNo++;
			}
			return lineNo;
		};

		return this.each(function() {
			var lineNo = 1;
			var textarea = $(this);

			/* Wrap the text area in the elements we need */
			textarea.wrap($("<div style='overflow:hidden;flex-grow:1;min-height:10em'></div>"));
			textarea.height('100%').css({'float': "right", 'line-height': '1.2em'}).attr('wrap', 'off');
			textarea.parent().
                prepend("<div class='lines' style='float:left;color:#ccc;text-align:right;line-height:1.2em'></div>").
                prepend("<div class='measure' style='white-space:nowrap;display:none'></div>");
			var linesDiv = textarea.parent().find(".lines");
			var measureDiv = textarea.parent().find(".measure");

			var scroll = function(tn) {
				var domTextArea = $(this)[0];
				var scrollTop = domTextArea.scrollTop;
				var clientHeight = domTextArea.clientHeight;
				linesDiv.css({
					'margin-top' : (-scrollTop) + "px"
				});
				lineNo = fillOutLines(linesDiv, scrollTop + clientHeight, lineNo);
                measureDiv.innerText = lineNo;
                linesDiv.width(window.getComputedStyle(measureDiv.get(0)).width);
                textarea.width(textarea.parent().width() - linesDiv.width() - 16);
			};
			/* React to the scroll event */
			textarea.scroll(scroll);
			$(window).resize(function() { textarea.scroll(); });
			/* We call scroll once to add the line numbers */
			textarea.scroll();

			/* React to textarea resize via css resize attribute. */
			var observer = new ResizeObserver(function(mutations) {
				textarea.scroll();
                textarea.parent().height(textarea.height());
			});
			observer.observe(textarea[0], {attributes: true});
			observer.observe(textarea.parents()[0], {attributes: true});
		});
	};

	$.fn.imageSelector = function(img) {
        const that = this;
        const readonly = !!this.attr('readonly');
        const processing = $("<div class='title'>").hide();
        const div = $('<div class="image-selector-container">').
            css('cursor', readonly ? 'inherit' : 'pointer').
            attr('readonly', readonly).
            append($("<div>").append($("<img>"))).
            append(processing).
            click(function() { !readonly && that.click() });
        div.insertBefore(this.hide());
        !readonly && processing.text('选择或粘贴图片').show();

        function finish(display, image, thumb) {
            div.find('img').get(0).src = display;
            that.get(0).thumb = thumb, that.get(0).image = image;
            processing.hide();
        }

        function onChange(file) {
            processing.text('处理中')
            if (!file) {
                that.attr('changed', '');
                finish('', null, null);
                return;
            }
            if (file.size < 1024 * 100) {
                that.attr('changed', 'true').attr('small', 'true');
                finish(URL.createObjectURL(file), file, null);
                return;
            }
            processing.show();
            const reader = new FileReader();
            const size = 300;
            reader.onload = function (e) {
                var img = document.createElement("img");
                img.onload = function (event) {
                    const canvas = document.createElement("canvas");
                    canvas.width = size; canvas.height = size;
                    const ctx = canvas.getContext("2d");
                    
                    if (img.width >= img.height * 3 || img.height >= img.width * 3) {
                        if (img.width > img.height) {
                            var w = size, h = size / img.width * img.height, x = 0, y = size - h, xs = 0, ys = y + 1;
                            var h0 = size - h, w0 = h0 / img.height * img.width, y0 = 0, x0 = (w0 - size) / 2;
                        } else {
                            var h = size, w = size / img.height * img.width, x = size - w, y = 0, xs = x + 1, ys = 0;
                            var w0 = size - w, h0 = w0 / img.width * img.height, x0 = 0, y0 = (h0 - size) / 2;
                        }
                        ctx.drawImage(img, -x0, -y0, w0, h0);
                        ctx.shadowColor = 'black';
                        ctx.shadowBlur = 10;
                        ctx.strokeRect(xs, ys, w, h);
                        ctx.shadowColor='rgba(0,0,0,0)';
                        ctx.drawImage(img, x, y, w, h);
                    } else {
                        if (img.width > img.height) {
                            var h = size, w = size / img.height * img.width, x = (w - size) / 2, y = 0;
                        } else {
                            var w = size, h = size / img.width * img.height, x = 0, y = (h - size) / 2;
                        }
                        ctx.drawImage(img, -x, -y, w, h);
                    }
                    canvas.toBlob(function(blob)  {
                        that.attr('changed', 'true');
                        finish(canvas.toDataURL('image/jpeg'), file, new File([blob], "thumb.jpg", { type: "image/jpeg" }));
                    }, 'image/jpeg');
                }
                img.src = e.target.result;
            }
            reader.readAsDataURL(file);
        }
        this.change(function() { onChange(that.get(0).files[0]) });
        img && (div.find('img').get(0).src = img);
        if (!readonly) {
            window.lastChangeImage = onChange;
        }
        return this;
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
        dialog.find('div.image-container-info').text((idx+1) + '/' + images.length);
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
    const fd = new FormData();    
    for (const k in args) fd.append(k, args[k]);
    const that = $(el);
    const rect = el.getBoundingClientRect();
    const loader = $("<div style='display:inline-block;text-align:center'>" + window.CONST_loaderHTML + "</div>").
        css('width', rect.width + 'px').
        css('margin', that.css('margin'));
    that.hide();
    loader.insertBefore(that);
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
                    alert('操作频繁，请在 ' + data.remains + ' 秒后重试') :
                    alert('发生错误: ' + data.msg + ' (' + data.code + ')');
                return;
            }
            f ? f(data, args) : location.reload();
        },
        error: function() {
            alert('网络错误');
        },
        complete: function () {
            that.show();
            loader.remove();
        },
    });
}

function wrapTagSearchInput(container) {
    if (container.wrapped) return;
    const maxTags = parseInt($(container).attr('max-tags') || '99');
    const readonly = !!$(container).attr('readonly');
    const div = document.createElement('div');
    div.className = 'tag-search-input';

    const el = document.createElement('input');
    el.readOnly = readonly;
    !readonly && (el.placeholder = '选择父记事');
    el.className = 'tag-box tag-search-box';
    el.style.outline = 'none';
    el.style.padding = '0 0.25em';
    el.style.minWidth = '2em';
    el.style.flexGrow = '1';

    const loader = $("<div class=tag-box style='min-width:2em;padding:0'>" + window.CONST_loaderHTML + "</div>").get(0);

    const info = $("<div class=tag-box style='font-size:80%;color:#aaa'></div>").get(0);

    const selected = {};

    function abbr(s) { return s.length < 16 ? s : s.substr(0, 16) + '...'; }

    function updateInfo() {
        const sz = Object.keys(selected).length;
        info.innerText = sz + '/' + maxTags;
        el.readOnly = sz > maxTags || readonly;
    }

    function select(src, fromHistory) {
        const tagID = parseInt(src.attr('tag-id')), tagText = src.attr('tag-text');
        if (!(tagID in selected) && Object.keys(selected).length < maxTags) {
            selected[tagID] = {'tag': tagText};
            const t = $("<div>").addClass('tag-box normal user-selected').attr('tag-id', tagID);
            t.append($("<span>").css('cursor', 'pointer').text(abbr(tagText)).click(function() {
                window.open('/ns:id:' + tagID);
            }));
            if (!readonly) {
                t.append($(window.CONST_closeSVG).click(function(ev) {
                    delete selected[tagID];
                    t.remove();
                    updateInfo();
                    el.focus();
                    ev.stopPropagation();
                }));
            }
            if (fromHistory) {
                t.insertBefore(src);
                src.remove();
            } else {
                t.insertBefore(el);
            }

            const history = JSON.parse(window.localStorage.getItem('tags-history') || '{}');
            history[tagID] = {'tag': tagText, 'ts': new Date().getTime()};
            if (Object.keys(history).length > 10) {
                var min = Number.MAX_VALUE, minID = 0;
                for (const k in history) {
                    if (history[k].ts < min) {
                        min = history[k].ts;
                        minID = k;
                    }
                }
                delete history[minID];
            }
            window.localStorage.setItem('tags-history', JSON.stringify(history));
            updateInfo();
        }

        if (fromHistory !== true) reset();
        el.value = '';
        el.focus();
    }

    function reset() {
        $(div).find('.candidate').remove();
        div.selector = 0;
        div.candidates = [];
        loader.style.display = 'none';
    }

    el.oninput = function(e){
        const val = this.value;
        const that = this;
        if (val.length < 1) {
            $(div).find('.candidate').remove();
            return;
        }
        if (this.timer) clearTimeout(this.timer);
        this.timer = setTimeout(function(){
            if (that.value != val) return;
            loader.style.display = '';
            $.get('/ns:search?n=100&q=' + encodeURIComponent(val), function(data) {
                if (that.value != val) return;

                reset();
                data.notes.forEach(function(tag, i) {
                    const t = $("<div>").
                        addClass('candidate tag-box ' + (i == 0 ? 'selected' : '')).
                        attr('tag-id', tag[0]).
                        attr('tag-text', tag[1]).
                        append($("<span>").text(tag[1]));
                    tag[2] > 0 && t.append($("<span class=children-count>").text(tag[2]));
                    $(div).append(t.click(function(ev) {
                        select(t);
                        ev.stopPropagation();
                    }));
                    div.candidates.push(t);
                })

                console.log(new Date(), val, data.notes.length);
            });
        }, 200);
    }
    el.onkeydown = function(e) {
        if ((e.keyCode == 9 || e.keyCode == 39) && div.candidates.length) {
            const current = div.selector;
            div.selector = (div.selector + (e.shiftKey ? -1 : 1) + div.candidates.length) % div.candidates.length;
            div.candidates[current].removeClass('selected');
            div.candidates[div.selector].addClass('selected');
            e.preventDefault();
        }
        if (e.keyCode == 13) {
            div.candidates.length && select(div.candidates[div.selector]);
            e.preventDefault();
        }
        if (e.keyCode == 8 && el.value.length == 0) {
            $(div).find('.user-selected:last .closer').click();
            e.preventDefault();
        }
        if (e.keyCode == 27) {
            el.value = '';
            reset();
            e.preventDefault();
        }
    }
    el.onblur = function() {
        this.blurtimer && clearTimeout(this.blurtimer);
        this.blurtimer = setTimeout(function() {
            el.value = '';
            reset();
        }, 1000);
    }
    el.onfocus = function() {
        this.blurtimer && clearTimeout(this.blurtimer);
    }
    container.onmouseup = function(ev) {
        el.focus();
        ev.preventDefault();
    }

    div.appendChild(el);
    div.appendChild(info);
    div.appendChild(loader);
    container.appendChild(div);
    reset();

    if (!readonly) {
        const history = JSON.parse(window.localStorage.getItem('tags-history') || '{}');
        for (const k in history) {
            const t = $("<div>").
                addClass('candidate tag-box').
                attr('tag-id', k).
                attr('tag-text', history[k].tag).
                append($('<span>').text(abbr(history[k].tag)))
            t.click(function(ev) {
                select(t, true);
                ev.stopPropagation();
            }).insertBefore(el);
            div.candidates.push(t);
        }
    }

    container.select = function(id, text) {
        select($("<div>").attr('tag-id', id).attr('tag-text', text));
    }

    for (var i = 0; ; i++) {
        const data = $(container).attr('data' + i);
        if (!data) break;
        container.select(data.split(',')[0], data.split(',')[1]);
        el.blur();
    }

    readonly && Object.keys(selected).length == 0 && (el.placeholder = '空');
    updateInfo();
    container.getTags = function() { return selected; }
    container.wrapped = true;
}

function openPreview(text) {
    const fd = new FormData(), w = window.open('', '_blank');
    fd.append('content', text);
    $.ajax({
        url: '/ns:action',
        data: fd,
        processData: false,
        contentType: false,
        type: 'POST',
        headers: { 'X-Ns-Action': 'preview' },
        success: function(data){ w.document.body.innerHTML = (data.content); },
        error: function() { alert('网络错误'); },
    });
}

document.onpaste = function (event) {
    var items = (event.clipboardData || event.originalEvent.clipboardData).items;
    for (var index in items) {
        var item = items[index];
        if (item.kind === 'file' && window.lastChangeImage) {
            window.lastChangeImage(item.getAsFile());
            break;
        }
    }
};
