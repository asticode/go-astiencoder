var astiencoder = {
    init () {
        // Handle keyboard
        document.addEventListener('keyup', this.onKeyUp.bind(this))

        // Open websocket
        this.openWebSocket({
            onopen: this.onopen.bind(this),
            onmessage: this.onmessage.bind(this)
        })
    },
    reset () {
        // Remove tags
        for (var name in this.tags) {
            delete(this.tags[name])
        }

        // Remove nodes
        for (var name in this.nodes) {
            delete(this.nodes[name])
        }
    },
    onopen () {
        // Fetch welcome
        this.sendHttp({
            method: 'GET',
            url: '/welcome',
            onsuccess: function(data) {
                // Reset
                this.reset()

                // Loop through nodes
                if (data.workflow) {
                    data.workflow.nodes.forEach(function(item) {
                        // Apply changes
                        this.apply(item.name, item)
                    }.bind(this))
                }
            }.bind(this)
        })
    },
    onmessage (name, payload, recording) {
        // Do nothing
        if (this.recording.loaded && !recording) return

        // Get rollback
        var rollback = false, n = false
        if (recording) {
            switch (name) {
                case 'astiencoder.node.continued':
                case 'astiencoder.node.paused':
                case 'astiencoder.node.stopped':
                    // Get node
                    n = this.nodes[payload]
                    if (!n) break
                    rollback = {}
                    
                    // Get name
                    switch (n.status) {
                        case 'paused':
                            rollback.name = 'astiencoder.node.paused'
                            break
                        case 'running':
                            rollback.name = 'astiencoder.node.continued'
                            break
                        case 'stopped':
                            rollback.name = 'astiencoder.node.stopped'
                            break
                    }
    
                    // Get payload
                    rollback.payload = payload
                    break
                case 'astiencoder.node.stats':
                    // Get node
                    n = this.nodes[payload.name]
                    if (!n) break
                    rollback = {}

                    // No stats
                    if (Object.keys(n.stats).length === 0) {
                        rollback = {
                            name: 'astiencoder.rollback.stats.remove',
                            payload: payload.name
                        }
                        break
                    }
    
                    // Get name
                    rollback.name = name
    
                    // Get payload
                    rollback.payload = {
                        name: payload.name,
                        stats: []
                    }

                    // Get stats
                    for (var stat in n.stats) {
                        var s = {}
                        for (var k in n.stats[stat]) {
                            s[k] = n.stats[stat][k]
                        }
                        rollback.payload.stats.push(s)
                    }
                    break
                case 'astiencoder.node.started':
                    // Get rollback
                    rollback = {
                        name: 'astiencoder.rollback.node.remove',
                        payload: payload.name
                    }
                    break
            }
        }

        // Apply
        switch (name) {
            case 'astiencoder.node.continued':
                this.apply(payload, {status: 'running'})
                break
            case 'astiencoder.node.paused':
                this.apply(payload, {status: 'paused'})
                break
            case 'astiencoder.node.stats':
                this.apply(payload.name, {stats: payload.stats})
                break
            case 'astiencoder.node.started':
                this.apply(payload.name, payload)
                break
            case 'astiencoder.node.stopped':
                this.apply(payload, {status: 'stopped'})
                break
        }
        return rollback
    },
    onKeyUp (event) {
        switch (event.code) {
            case 'ArrowLeft':
                this.onRecordingPreviousClick()
                break
            case 'ArrowRight':
                this.onRecordingNextClick()
                break
        }
    },

    /* recording */
    recording: new Proxy({
        cursorNexts: 0,
        cursorPreviouses: 0,
        nexts: [],
        previouses: [],
        parse (line) {
            // Split line
            const items = line.split(',')

            // Not enough items
            if (items.length !== 3) return false

            // Create data
            const d = {}
            if (items[0] !== '') d.time = new Date(items[0] * 1000)
            if (items[1] !== '') d.name = items[1]
            if (items[2] !== '') {
                const p = atob(items[2])
                if (p !== "null") d.payload = JSON.parse(p)
            }
            return d
        },
        apply (list, direction) {
            // Loop through items
            var rollbacks = []
            list.items.forEach(function(item) {
                switch (item.name) {
                    case 'astiencoder.rollback.node.remove':
                        delete astiencoder.nodes[item.payload]
                        break
                    case 'astiencoder.rollback.stats.remove':
                        // Get node
                        const n = astiencoder.nodes[item.payload]
                        if (!n) break

                        // Loop through stats
                        for (var stat in n.stats) {
                            delete n.stats[stat]
                        }
                        break
                    default:
                        const r = astiencoder.onmessage(item.name, item.payload, true)
                        if (r) rollbacks.push(r)
                        break
                }
            })

            // Add previous
            if (direction === 'next' && this.cursorNexts === this.previouses.length) {
                if (rollbacks.length > 0) this.previouses.unshift({
                    items: rollbacks,
                    time: this.currentTime
                })
            }

            // Update time
            this.updateTime(list.time)
        },
        updateTime (t) {
            this.currentTime = t
            document.getElementById('progress').value = ((t.getTime() - this.from.getTime()) / this.duration) * 100
            document.getElementById('time').innerText = t.getHours().toString().padStart(2, '0') + ':' + t.getMinutes().toString().padStart(2, '0') + ':' + t.getSeconds().toString().padStart(2, '0')
        },
        next () {
            // No nexts
            if (this.nexts.length <= this.cursorNexts) return false

            // Get nexts
            return this.nexts[this.cursorNexts]
        },
        applyNext () {
            // Get nexts
            const nexts = this.next()
            if (!nexts) return

            // Apply
            this.apply(nexts, 'next')

            // Update cursors
            this.cursorNexts++
            if (this.cursorPreviouses > 0) this.cursorPreviouses--
        },
        previous () {
            // No previouses
            if (this.previouses.length <= this.cursorPreviouses) return false

            // Get previouses
            return this.previouses[this.cursorPreviouses]
        },
        applyPrevious () {
            // Get previouses
            const previouses = this.previous()
            if (!previouses) return

            // Apply
            this.apply(previouses, 'previous')

            // Update cursors
            this.cursorPreviouses++
            if (this.cursorNexts > 0) this.cursorNexts--
        }
    }, {
        set: function(obj, prop, value) {
            // Nothing changed
            if (typeof obj[prop] !== 'undefined' && obj[prop] === value) return

            // Switch on prop
            switch (prop) {
                case 'loaded':
                    if (value) document.querySelector('footer').classList.add('recording-loaded')
                    else document.querySelector('footer').classList.remove('recording-loaded')
                    break
            }

            // Store value
            obj[prop] = value
            return true
        }
    }),
    onRecordingLoadClick () {
        document.querySelector('#recording-load input').click()
    },
    onRecordingLoadChange (event) {
        // No file
        if (event.target.files.length === 0) return

        // Create reader
        const r = new FileReader()
        r.addEventListener('load', () => {
            // Parse lines
            var lines = r.result.split(/\r\n|\n/)

            // Remove last line if empty
            if (lines[lines.length -1] === '') lines.pop()

            // Reset
            this.reset()

            // Update recording
            this.recording.loaded = true

            // No lines
            if (lines.length === 0) return
            
            // Get init
            var n = this.recording.parse(lines[0])
            lines.shift()

            // Loop through nodes
            n.payload.nodes.forEach(function(item) {
                // Apply changes
                this.apply(item.name, item)
            }.bind(this))

            // Update from
            this.recording.from = n.time
            
            // Loop through lines
            var nexts = []
            var indexed = {}
            while (1 === 1) {
                // No more lines
                if (lines.length === 0) {
                    if (nexts.length > 0) this.recording.nexts.push({
                        items: nexts,
                        time: nexts[0].time
                    })
                    break
                }

                // Get next
                n = this.recording.parse(lines[0])
    
                // Get indexed key
                var k = ''
                switch (n.name) {
                    case 'astiencoder.node.continued':
                    case 'astiencoder.node.paused':
                    case 'astiencoder.node.stopped':
                        k = 'status | ' + n.payload
                        break
                    case 'astiencoder.node.stats':
                        k = 'stats | ' + n.payload.name
                    case 'astiencoder.node.started':
                        k = 'status | ' + n.payload.name
                        break
                    default:
                        lines.shift()
                        continue
                }
    
                // Same event type is being processed for same node
                if (indexed[k]) {
                    if (nexts.length > 0) this.recording.nexts.push({
                        items: nexts,
                        time: nexts[0].time
                    })
                    nexts = []
                    indexed = {}
                    continue
                }
                indexed[k] = true
                lines.shift()
    
                // Append next
                nexts.push(n)
            }

            // No nexts
            if (this.recording.nexts.length === 0) return

            // Update duration
            this.recording.duration = this.recording.nexts[this.recording.nexts.length - 1].time.getTime() - this.recording.from.getTime()

            // Update time
            this.recording.updateTime(this.recording.from)
        });
        r.readAsText(event.target.files[0])
    },

    onRecordingUnloadClick () {
        // Update recording
        this.recording.loaded = false

        // On open
        this.onopen()
    },
    onRecordingNextClick () {
        // No recording
        if (!this.recording.loaded) return

        // Apply next
        this.recording.applyNext()
    },
    onRecordingPreviousClick () {
        // No recording
        if (!this.recording.loaded) return

        // Apply previous
        this.recording.applyPrevious()
    },
    onRecordingSeek (e) {
        // No recording
        if (!this.recording.loaded) return

        // Get seek time
        const t = new Date(e.target.value / 100 * this.recording.duration + this.recording.from.getTime())

        // Seek
        if (t.getTime() > this.recording.currentTime.getTime()) {
            while (1 === 1) {
                // Get next
                const nexts = this.recording.next()
                if (!nexts) break

                // Invalid time
                if (nexts.time.getTime() > t.getTime()) break

                // Apply next
                this.recording.applyNext()
            }
        } else {
            while (1 === 1) {
                // Get previous
                const previouses = this.recording.previous()
                if (!previouses) break

                // Invalid time
                if (previouses.time.getTime() < t.getTime()) break

                // Apply previous
                this.recording.applyPrevious()
            }
        }
    },

    /* tags */
    tags: new Proxy({}, {
        deleteProperty: function(obj, prop) {
            // Get value
            const value = obj[prop]
            if (!value) return

            // Delete wrapper
            document.getElementById('tags').removeChild(value.dom.w)

            // Delete prop
            delete(obj[prop])
        },
        set: function(obj, prop, value) {
            // Tag already exists
            if (typeof obj[prop] !== 'undefined') return

            // Create tag
            var t = {
                _key: prop,
                dom: {},
            }

            // Create wrapper
            t.dom.w = document.createElement('div')

            // Append wrapper in alphabetical order
            var p = null
            for (var name in astiencoder.tags) {
                const i = astiencoder.tags[name]
                if (t._key < i._key && (!p || p._key > i._key)) p = i
            }
            if (p) document.getElementById('tags').insertBefore(t.dom.w, p.dom.w)
            else document.getElementById('tags').appendChild(t.dom.w)

            // Add show
            const _s = document.createElement('i')
            _s.className = 'fa fa-eye'
            _s.onclick = function() {
                astiencoder.tags[prop].hide = false
                astiencoder.tags[prop].show = !astiencoder.tags[prop].show
            }
            t.dom.w.appendChild(_s)

            // Add label
            const _l = document.createElement('span')
            _l.innerText = prop
            t.dom.w.appendChild(_l)

            // Add hide
            const _h = document.createElement('i')
            _h.className = 'fa fa-eye-slash'
            _h.onclick = function() {
                astiencoder.tags[prop].show = false
                astiencoder.tags[prop].hide = !astiencoder.tags[prop].hide
            }
            t.dom.w.appendChild(_h)

            // Store value
            obj[prop] = new Proxy(t, {
                set: function(obj, prop, value) {
                    // Nothing changed
                    if (typeof obj[prop] !== 'undefined' && obj[prop] === value) return

                    // Store value
                    obj[prop] = value

                    // Switch on prop
                    switch (prop) {
                        case 'hide':
                            if (value) _h.classList.add('active')
                            else _h.classList.remove('active')
                            break
                        case 'show':
                            if (value) _s.classList.add('active')
                            else _s.classList.remove('active')
                            break
                    }

                    // Refresh tags
                    astiencoder.refreshTags()
                    return true
                }
            })
            return true
        }
    }),
    refreshTags () {
        for (var name in this.nodes) {
            this.refreshTagsForNode(name)
        }
    },
    refreshTagsForNode (name) {
        // Index tags
        var hides = {}
        var shows = {}
        for (var tag in this.tags) {
            if (this.tags[tag].hide) hides[tag] = true
            if (this.tags[tag].show) shows[tag] = true
        }

        // Check node
        var hide = false
        var show = false
        for (var tag in this.nodes[name].tags) {
            if (hides[tag]) hide = true
            else if (shows[tag]) show = true
        }

        // Update node
        if (hide) this.nodes[name].notInTags = true
        else if (show) this.nodes[name].notInTags = false
        else this.nodes[name].notInTags = Object.keys(shows).length > 0
    },
    onResetAllTags () {
        for (var name in astiencoder.tags) {
            astiencoder.tags[name].hide = false
            astiencoder.tags[name].show = false
        }
    },

    /* nodes */
    nodes: new Proxy({}, {
        deleteProperty: function(obj, prop) {
            // Get value
            const value = obj[prop]
            if (!value) return

            // Delete wrapper
            document.getElementById('nodes').removeChild(value.dom.w)

            // Delete prop
            delete(obj[prop])
        },
        set: function(obj, prop, value) {
            // Node already exists
            if (typeof obj[prop] !== 'undefined') return

            // Create node
            var n = {
                _key: value.label,
                dom: {},
            }

            // We need to store locally the node name since it's used by refreshTagsForNode
            const nodeName = prop

            // Create wrapper
            n.dom.w = document.createElement('div')

            // Append wrapper in label alphabetical order
            var p = null
            for (var name in astiencoder.nodes) {
                const i = astiencoder.nodes[name]
                if (n._key < i._key && (!p || p._key > i._key)) p = i
            }
            if (p) document.getElementById('nodes').insertBefore(n.dom.w, p.dom.w)
            else document.getElementById('nodes').appendChild(n.dom.w)

            // Add children
            n.children = new Proxy({}, {
                deleteProperty: function(obj, prop) {
                    // Delete prop
                    delete(obj[prop])

                    // Refresh not active
                    astiencoder.refreshNotActive(n)
                },
                set: function(obj, prop, value) {
                    // Nothing changed
                    if (typeof obj[prop] !== 'undefined' && obj[prop] === value) return

                    // Store value
                    obj[prop] = value

                    // Refresh not active
                    astiencoder.refreshNotActive(n)
                    return true
                }
            })

            // Add label
            const _l = document.createElement('div')
            _l.className = 'label'
            n.dom.w.appendChild(_l)

            // Add name
            const _n = document.createElement('div')
            _n.className = 'name'
            n.dom.w.appendChild(_n)

            // Add parents
            n.parents = new Proxy({}, {
                deleteProperty: function(obj, prop) {
                    // Delete prop
                    delete(obj[prop])

                    // Refresh not active
                    astiencoder.refreshNotActive(n)
                },
                set: function(obj, prop, value) {
                    // Nothing changed
                    if (typeof obj[prop] !== 'undefined' && obj[prop] === value) return

                    // Store value
                    obj[prop] = value

                    // Refresh not active
                    astiencoder.refreshNotActive(n)
                    return true
                }
            })

            // Add stats
            const _ss = document.createElement('table')
            _ss.className = 'stats'
            n.dom.w.appendChild(_ss)
            n.stats = new Proxy({}, {
                deleteProperty: function(obj, prop) {
                    // Stat doesn't exists
                    if (typeof obj[prop] === 'undefined') return

                    // Remove row
                    _ss.removeChild(obj[prop].dom.r)

                    // Delete prop
                    delete(obj[prop])
                },
                set: function(obj, prop, value) {
                    // Stat already exists
                    if (typeof obj[prop] !== 'undefined') return

                    // Create stats
                    var s = {
                        _key: value.label,
                        dom: {}
                    }

                    // Create row
                    s.dom.r = document.createElement('tr')

                    // Append row in label alphabetical order
                    var p = null
                    for (var name in obj) {
                        const i = obj[name]
                        if (s._key < i._key && (!p || p._key > i._key)) p = i
                    }
                    if (p) _ss.insertBefore(s.dom.r, p.dom.r)
                    else _ss.appendChild(s.dom.r)

                    // Add label
                    const _c1 = document.createElement('td')
                    s.dom.r.appendChild(_c1)

                    // Add value
                    const _c2 = document.createElement('td')
                    s.dom.r.appendChild(_c2)
                    const _v = document.createElement('span')
                    _c2.appendChild(_v)

                    // Add unit
                    const _u = document.createElement('span')
                    _c2.appendChild(_u)

                    // Store stat
                    obj[prop] = new Proxy(s, {
                        set: function(obj, prop, value) {
                            // Nothing changed
                            if (typeof obj[prop] !== 'undefined' && obj[prop] === value) return

                            // Switch on prop
                            switch (prop) {
                                case 'label':
                                    _c1.innerText = value + ':'
                                    break
                                case 'unit':
                                    _u.innerText = value
                                    break
                                case 'value':
                                    _v.innerText = value
                                    break
                            }
        
                            // Store value
                            obj[prop] = value
                            return true
                        }
                    })
                    return true
                }
            })

            // Add tags
            n.tags = new Proxy({}, {
                set: function(obj, prop, value) {
                    // Nothing changed
                    if (typeof obj[prop] !== 'undefined' && obj[prop] === value) return

                    // Add tag
                    // If tag exists, it will do nothing
                    astiencoder.tags[prop] = true

                    // Store value
                    obj[prop] = value

                    // Refresh tags
                    astiencoder.refreshTagsForNode(nodeName)
                    return true
                }
            })

            // Store node
            obj[prop] = new Proxy(n, {
                set: function(obj, prop, value) {
                    // Nothing changed
                    if (typeof obj[prop] !== 'undefined' && obj[prop] === value) return

                    // Switch on prop
                    var refreshNotActive = false
                    switch (prop) {
                        case 'label':
                            _l.innerText = value
                            break
                        case 'name':
                            _n.innerText = value
                            break
                        case 'notInSearch':
                            if (value) n.dom.w.classList.add('not-in-search')
                            else n.dom.w.classList.remove('not-in-search')
                            break
                        case 'notInTags':
                            if (value) n.dom.w.classList.add('not-in-tags')
                            else n.dom.w.classList.remove('not-in-tags')
                            break
                        case 'status':
                            if (obj[prop] !== value) {
                                // Update class
                                n.dom.w.classList.remove(obj[prop])
                                n.dom.w.classList.add(value)

                                // Update tags
                                delete(n.tags[obj[prop]])
                                n.tags[value] = true

                                // Refresh not active
                                refreshNotActive = true
                            }
                            break
                    }

                    // Store value
                    obj[prop] = value

                    // Refresh not active
                    // It needs the new value to be set
                    if (refreshNotActive) astiencoder.refreshNotActive(n)
                    return true
                }
            })
            return true
        }
    }),
    refreshNotActive (n) {
        if (Object.keys(n.children).length === 0
            && Object.keys(n.parents).length === 0
            && n.status === 'stopped') n.dom.w.classList.add('not-active')
        else n.dom.w.classList.remove('not-active')
    },
    apply (name, payload) {
        // Add node
        // If node already exists, it will do nothing
        this.nodes[name] = payload

        // Children
        if (payload.children) {
            payload.children.forEach(function(item) {
                this.nodes[name].children[item] = true
            }.bind(this))
        }

        // Description
        if (payload.description) this.nodes[name].description = payload.description

        // Label
        if (payload.label) this.nodes[name].label = payload.label

        // Name
        if (payload.name) this.nodes[name].name = payload.name

        // Parents
        if (payload.parents) {
            payload.parents.forEach(function(item) {
                this.nodes[name].parents[item] = true
            }.bind(this))
        }

        // Stats
        if (payload.stats) {
            // Loop through stats
            payload.stats.forEach(function(item) {
                // Add stat
                // If stat already exists, it will do nothing
                this.nodes[name].stats[item.label] = item

                // Description
                if (item.description) this.nodes[name].stats[item.label].description = item.description

                // Label
                if (item.label) this.nodes[name].stats[item.label].label = item.label

                // Unit
                if (item.unit) this.nodes[name].stats[item.label].unit = item.unit

                // Value
                if (typeof item.value !== 'undefined') {
                    var v = item.value
                    if (!isNaN(parseFloat(v))) {
                        v = parseFloat(item.value).toFixed(2)
                        if (v < 10 && v >= 0) v = '0' + v
                        else if (v > -10 && v < 0) v = '-0' + (-v)
                        if (this.nodes[name].stats[item.label].unit === '%') {
                            if (v > 1000) v = '+∞'
                            else if (v < -1000) v = '-∞'
                        }
                    }
                    this.nodes[name].stats[item.label].value = v
                }
            }.bind(this))
        }

        // Status
        if (payload.status) this.nodes[name].status = payload.status

        // Tags
        if (payload.tags) {
            // Loop through tags
            payload.tags.forEach(function(item) {
                this.nodes[name].tags[item] = true
            }.bind(this))
        }
    },

    /* search */
    onSearch (event) {
        // Loop through nodes
        for (var name in this.nodes) {
            this.nodes[name].notInSearch = event.target.value !== ''
                && this.nodes[name].label.toLowerCase().search(event.target.value.toLowerCase()) === -1
                && this.nodes[name].name.toLowerCase().search(event.target.value.toLowerCase()) === -1
        }
    },

    /* helpers */
    sendHttp (options) {
        const req = new XMLHttpRequest()
        req.onreadystatechange = function() {
            if (this.readyState === XMLHttpRequest.DONE) {
                var data = null
                try {
                    if (this.responseText !== '') data = JSON.parse(this.responseText)
                } catch (e) {}
                if (this.status >= 200 && this.status < 300) {
                    if (options.onsuccess) options.onsuccess(data)
                } else {
                    if (options.onerror) options.onerror(data)
                }
            }
        }
        req.open(options.method, options.url, true)
        req.send(options.payload)
    },
    openWebSocket (options) {
        // Make sure to close the websocket when page closes
        if (!this.unloadHandled) {
            if (this.ws) {
                this.ws.close()
                this.ws = null
            }
            this.unloadHandled = true
        }

        // Send health request
        this.sendHttp({
            url: '/ok',
            method: 'GET',
            onerror: function() {
                // Make sure to reconnect when server is down
                setTimeout(function() {
                    this.openWebSocket(options)
                }.bind(this), 1000)
            }.bind(this),
            onsuccess: function() {
                // Create websocket
                this.ws = new WebSocket((window.location.protocol === 'https:' ? 'wss://' : 'ws://') + window.location.host + '/websocket')
    
                // Handle open
                var pingInterval = null
                this.ws.onopen = function() {
                    // Make sure to ping
                    pingInterval = setInterval(function() {
                        this.sendWebSocket('ping')
                    }.bind(this), 50*1e3)
    
                    // Open callback
                    options.onopen()
                }.bind(this)
    
                // Handle close
                this.ws.onclose = function() {
                    // Cancel ping
                    clearInterval(pingInterval)
    
                    // Reconnect
                    setTimeout(function() {
                        this.openWebSocket(options)
                    }.bind(this), 1000)
                }.bind(this)
    
                // Handle message
                this.ws.onmessage = function(event) {
                    var data = JSON.parse(event.data)
                    options.onmessage(data.event_name, data.payload)
                }.bind(this)
            }.bind(this)
        })
    },
    sendWebSocket (name, payload) {
        if (!this.ws) return
        var d = {event_name: name}
        if (payload) d.payload = payload
        this.ws.send(JSON.stringify(d))
    }
}

astiencoder.init()
