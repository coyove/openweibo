window.CONST_closeSVG = "<svg class='svg16 closer' viewBox='0 0 16 16'><circle cx=8 cy=8 r=8 fill='#aaa' /><path d='M 5 5 L 11 11' stroke=white stroke-width=3 fill=transparent /><path d='M 11 5 L 5 11' stroke=white stroke-width=3 fill=transparent /></svg>";
window.CONST_tickSVG = "<svg class='svg16' viewBox='0 0 16 16'><circle cx=8 cy=8 r=8 fill='green' /><path fill=white stroke=transparent d='M 4.139 6.749 L 2.235 8.848 L 6.491 13.156 L 13.534 6.429 L 11.657 4.382 L 6.781 9.244 L 4.139 6.749 Z' /></svg>";
window.CONST_starSVG = "<svg class='svg16 starer' viewBox='0 0 16 16'><circle cx=8 cy=8 r=8 fill='#aaa' /><path d='M 8.065 2.75 L 9.418 6.642 L 13.537 6.726 L 10.254 9.215 L 11.447 13.159 L 8.065 10.806 L 4.683 13.159 L 5.876 9.215 L 2.593 6.726 L 6.712 6.642 Z' fill=white /></svg>";
window.CONST_loaderHTML = "<div class=lds-dual-ring></div>";

function clickAjax(el, path, argsf, f, config) {
    $(el).click(function() {
        if (config && config.ask) {
            if (!confirm(config.ask)) return;
        }

        var url = path + '?ajax=1';
        const args = argsf(el);
        for (const k in args) url += '&' + encodeURIComponent(k) + '=' + encodeURIComponent(args[k]);
        const that = $(this);
        const rect = this.getBoundingClientRect();
        const loader = $("<div style='display:inline-block;text-align:center'>" + window.CONST_loaderHTML + "</div>").
            css('width', rect.width + 'px').
            css('margin', that.css('margin'));
        that.hide();
        loader.insertBefore(that);
        $.post(url, function(data) {
            if (!data.success) {
                var i18n = ({
"INTERNAL_ERROR": "服务器错误",
"MODS_REQUIRED": "无管理员权限",
"TAG_PENDING_REVIEW": "标签审核中",
"TAG_LOCKED": "标签已锁定",
"INVALID_CONTENT": "无效内容，过长或过短",
"TOO_MANY_PARENTS": "父标签过多，最多8个",
"TAG_ALREADY_EXISTS": "标签重名",
"ILLEGAL_SELF_APPROVE": "无法审核自己的修改",
"INVALID_ACTION": "请求错误",
                })[data.code];
                alert('发生错误: ' + i18n + ' (' + data.code + ')');
                return;
            }
            f(data, args);
        }).fail(function() {
            alert('网络错误');
        }).always(function() {
            that.show();
            loader.remove();
        });
    })
}

window.searchParams = new URLSearchParams(window.location.search)

window.onload = function() {
    function teShowEdit(tagID) {
        if (tagID) {
            window.searchParams.set('edittagid', tagID);
            window.history.replaceState({}, '标签编辑 ' + tagID, '?' + window.searchParams.toString());
        }
        $("#list").hide();
        $("#page").hide();
        const tab = $("#edit").show().html('');
        tab.append($("<tr><td><div class=display><div class='button tag-edit-button'>" + window.CONST_closeSVG + "</div></div></td></tr>"));
        tab.find('.button').click(function() {
            window.searchParams.delete('edittagid');
            window.history.replaceState({}, '标签管理', '?' + window.searchParams.toString());
            tab.hide();
            $("#list").show();
            $("#page").show();
            if (tagID) $("#tag" + tagID).get(0).scrollIntoView();
        });
        return tab;
    }
    $('.tag-edit').each(function(_, el) {
        const input = $(el).find('.tag-edit-button.tag-edit-button-update');
        const tagID = $(el).attr('tag-id');
        const tagData = JSON.parse($(el).find('.tag-data').html() || '{}');
        const path = '/tag/manage/action';
        function reload() {
            window.searchParams.set('edittagid', tagID);
            location.href = '?' + window.searchParams.toString();
        }
        input.click(function() {
            const tab = teShowEdit(tagID);
            tab.append($("<tr><td class=small>ID</td><td><div class=display>" + (tagData.I || '新标签') + "</div></td></tr>"));
            var tr = $("<tr><td class=small>标签名</td><td><div class=display><input class=tag-edit-name /></div></td></tr>");
            var trInput = tr.find('input').val(tagData.O);
            tr.find('.display').append($("<div class='tag-box button'><span>更新</span></div>"));
            var btnUpdate = tr.find('.display .button').hide();
            clickAjax(btnUpdate, path, function() {
                return {
                    'action': 'update',
                    'id': tagID,
                    'text': trInput.val(),
                    'description': trDesc.val(),
                    'parents': JSON.stringify(parentsSelector.getTags()),
                };
            }, reload);
            tab.append(tr);

            var tr = $("<tr><td class=small>描述</td><td><div class=display><input class=tag-edit-desc /></div></td></tr>");
            var trDesc = tr.find('input').val(tagData.D);
            tab.append(tr);

            if (tagData.pr) {
                var tr = $("<tr><td class=small>标签名（待审核）</td><td><div class=display><input class=tag-edit-name readonly/></div></td></tr>");
                tr.find('input').val(tagData.pn);
                tr.find('.display').append($("<div class='tag-box button' tag=approve><span>通过</span></div>")).
                    append($("<div class='tag-box button' tag=reject><span>驳回</span></div>"));
                clickAjax(tr.find('[tag=approve]'), path, function() { return {'action': 'approve', 'id': tagID} }, reload);
                clickAjax(tr.find('[tag=reject]'), path, function() { return {'action': 'reject', 'id': tagID} }, reload);
                tab.append(tr);

                var tr = $("<tr><td class=small>描述（待审核）</td><td><div class=display><input class=tag-edit-name readonly/></div></td></tr>");
                tr.find('input').val(tagData.pd);
                tab.append(tr);
            }

            var trParents = $("<tr><td class=small>父标签</td><td><div class=display></div></td></tr>"), parentsSelector;
            trParents.find('.display').append($(window.CONST_loaderHTML));
            tab.append(trParents);
            $.get('/tag/search?n=100&ids=' + (tagData.P || []).join(','), function(data) {
                trParents.find('.display').html('').
                    append($("<div max-tags=8 class='tag-search-input-container border1' style='width:100%'></div>"));
                parentsSelector = trParents.find('.tag-search-input-container').get(0);
                parentsSelector.onclicktag = function(id) { window.open('/tag/manage?edittagid=' + id); }
                data.tags.forEach(function(t, i) { $(parentsSelector).attr('tag-data' + i, t[0] + ',' + t[1]) });
                wrapTagSearchInput(parentsSelector );
                if (!tagData.pr) btnUpdate.show();
            })

            tab.append($("<tr><td class=small>子标签</td><td><div class=display><a href='?pid=" + tagData.I + "'>查看</a></div></td></tr>"));
            tab.append($("<tr><td class=small>变更历史</td><td><div class=display><a href='/tag/history?desc=1&id=" + tagData.I + "'>查看</a></div></td></tr>"));

            var tr = $("<tr><td class=small>状态</td><td><div class=display><span>" + (tagData.L ? '<b>锁定中</b>' : '正常' ) + "&nbsp;</span></div></td></tr>")
            tr.find('.display').append($("<div class='tag-edit-button button'><span class=li_lock></span></div>"));
            clickAjax(tr.find('.display .button'), path, function(btn) {
                return {'action': tagData.L ? 'unlock' : 'lock', 'id': tagID};
            }, reload);
            tab.append(tr);

            tab.append($("<tr><td class=small>创建者</td><td><div class=display>" + tagData.U + "</div></td></tr>"));
            tab.append($("<tr><td class=small>创建时间</td><td><div class=display>" + new Date(tagData.C || 0).toLocaleString() + "</div></td></tr>"));
            tab.append($("<tr><td class=small>最近修改人</td><td><div class=display>" + (tagData.M || '') + "</div></td></tr>"));
            tab.append($("<tr><td class=small>最近审核人</td><td><div class=display>" + (tagData.R || '') + "</div></td></tr>"));
            tab.append($("<tr><td class=small>修改时间</td><td><div class=display>" + new Date(tagData.u || 0).toLocaleString() + "</div></td></tr>"));

            var tr = $("<tr><td class=small></td><td><div class=display></div></td></tr>")
            tr.find('.display').append($("<div class='tag-box button alert'><span>删除标签</span></div>"));
            clickAjax(tr.find('.display .button'), path, function() {
                return {'action': 'delete', 'id': tagID};
            }, function(data) {
                window.searchParams.delete('edittagid');
                location.href = '?' + window.searchParams.toString();
            }, {'ask': '确认删除 ' + input.val()});
            tab.append(tr);
        });
    });

    $('#tag-edit-new-tag').click(function() {
        const tab = teShowEdit();
        var tr = $("<tr><td class=small>标签名</td><td><div class=display><input class=tag-edit-name placeholder='32字符' /></div></td></tr>");
        var trInput = tr.find('input');
        tr.find('.display').append($("<div class='tag-box button'><span>创建</span></div>"));
        clickAjax(tr.find('.display .button'), '/tag/manage/action', function() {
            return {'action': 'create', 'text': trInput.val(), 'description': trDesc.val(), 'parents': JSON.stringify(parents.get(0).getTags())};
        }, function(data) {
            location.href = '?sort=0&desc=1';
        });
        tab.append(tr);

        var tr = $("<tr><td class=small>描述</td><td><div class=display><input class=tag-edit-desc placeholder='500字符' /></div></td></tr>");
        var trDesc = tr.find('input');
        tab.append(tr);

        var tr = $("<tr><td class=small>父标签</td><td><div class=display></div></td></tr>");
        tr.find('.display').append($("<div max-tags=8 class='tag-search-input-container border1'></div>").css('width', '100%'));
        var parents = tr.find('.tag-search-input-container');
        wrapTagSearchInput(parents.get(0));
        tab.append(tr);
    });

    if (window.searchParams.has('edittagid')) 
        $('#tag' + window.searchParams.get('edittagid')).find('.tag-edit-button.tag-edit-button-update').click();

    $('.tag-search-input-container').each(function(_, container) { wrapTagSearchInput(container) });
    function wrapTagSearchInput(container) {
        if (container.wrapped) return;
        const editable = $(container).attr('edit') == 'edit';
        const maxTags = parseInt($(container).attr('max-tags') || '99');
        const div = document.createElement('div');
        div.className = 'tag-search-input';

        var onClickTag = function() {};
        if ($(container).attr('onclicktag')) 
            var onClickTag = function(id) { eval('var id = ' + id + '; ' + $(container).attr('onclicktag')) };
        if (container.onclicktag)
            var onClickTag = container.onclicktag;

        const el = document.createElement('div');
        el.setAttribute('contenteditable', true);
        el.className = 'tag-box tag-search-box';
        el.style.outline = 'none';
        el.style.minWidth = '2em';
        el.style.flexGrow = '1';
        el.style.justifyContent = 'left';

        const loader = $("<div class=tag-box style='min-width:2em;padding:0'>" + window.CONST_loaderHTML + "</div>").get(0);

        const info = $("<div class=tag-box style='font-size:80%;color:#aaa'></div>").get(0);

        const selected = {};

        function updateInfo() {
            const sz = Object.keys(selected).length;
            info.innerText = sz + '/' + maxTags;
            el.setAttribute('contenteditable', sz < maxTags);
        }

        function select(src, fromHistory) {
            const tagID = parseInt(src.attr('tag-id'));
            if (!(tagID in selected) && Object.keys(selected).length < maxTags) {
                selected[tagID] = {'tag': src.text()};
                const t = $("<div>").addClass('tag-box normal user-selected').attr('tag-id', tagID);
                if (editable) {
                    t.append($(window.CONST_starSVG).click(function(ev){
                        t.toggleClass('tag-required');
                        selected[tagID].required = t.hasClass('tag-required');
                    }));
                }
                t.append($("<span>").css('cursor', 'pointer').text(src.text()).click(function() { onClickTag(tagID) }));
                t.append($(window.CONST_closeSVG).click(function(ev) {
                    delete selected[tagID];
                    t.remove();
                    updateInfo();
                    el.focus();
                    ev.stopPropagation();
                }));
                if (fromHistory) {
                    t.insertBefore(src);
                    src.remove();
                } else {
                    t.insertBefore(el);
                }

                const history = JSON.parse(window.localStorage.getItem('tags-history') || '{}');
                history[tagID] = {'tag': src.text(), 'ts': new Date().getTime()};
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
            el.innerText = '';
            el.focus();
        }

        function reset() {
            $(div).find('.candidate').remove();
            div.selector = 0;
            div.candidates = [];
            loader.style.display = 'none';
        }

        el.oninput = function(e){
            const val = this.textContent;
            const that = this;
            if (val.length < 1) {
                $(div).find('.candidate').remove();
                return;
            }
            if (this.timer) clearTimeout(this.timer);
            this.timer = setTimeout(function(){
                if (that.textContent != val) return;
                loader.style.display = '';
                $.get('/tag/search?n=100&q=' + encodeURIComponent(val), function(data) {
                    if (that.textContent != val) return;
                    
                    reset();
                    data.tags.forEach(function(tag, i) {
                        const t = $("<div>").
                            addClass('candidate tag-box ' + (i == 0 ? 'selected' : '')).
                            attr('tag-id', tag[0]).
                            append($("<span>").text(tag[1]));
                        $(div).append(t.click(function(ev) {
                            select(t);
                            ev.stopPropagation();
                        }));
                        div.candidates.push(t);
                    })

                    console.log(new Date(), val, data.tags.length);
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
                if (div.candidates.length) {
                    select(div.candidates[div.selector]);
                }
                e.preventDefault();
            }
            if (e.keyCode == 8 && el.textContent.length == 0) {
                $(div).find('.user-selected:last .closer').click();
                e.preventDefault();
            }
            if (e.keyCode == 27) {
                el.innerHTML = '';
                reset();
                e.preventDefault();
            }
        }
        el.onblur = function() {
            if (this.blurtimer) clearTimeout(this.blurtimer);
            this.blurtimer = setTimeout(function() {
                el.innerHTML = '';
                reset();
            }, 1000);
        }
        el.onfocus = function() {
            if (this.blurtimer) clearTimeout(this.blurtimer);
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

        const history = JSON.parse(window.localStorage.getItem('tags-history') || '{}');
        for (const k in history) {
            const t = $("<div>").
                addClass('candidate tag-box').
                attr('tag-id', k).
                append($("<span>").text(history[k].tag));
            t.click(function(ev) {
                select(t, true);
                ev.stopPropagation();
            }).insertBefore(el);
            div.candidates.push(t);
        }

        for (var i = 0; ; i++) {
            const data = $(container).attr('tag-data' + i);
            if (!data) break;
            select($("<div>").attr('tag-id', data.split(',')[0]).text(data.split(',')[1]));
            el.blur();
        }

        updateInfo();
        container.getTags = function() { return selected; }
        container.wrapped = true;
    }
}
