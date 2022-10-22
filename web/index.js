var astiencoder = {
    init () {
        // Handle keyboard
        document.addEventListener('keydown', this.onKeyDown.bind(this))
        document.addEventListener('keyup', this.onKeyUp.bind(this))

        // Get params
        const params = new URLSearchParams(window.location.search)

        // Init nodes position refresher
        this.initNodesPositionRefresher()

        // Get urls
        this.websocketUrl = params.get("websocket_url")
        this.welcomeUrl = params.get("welcome_url")

        // Open websocket
        if (this.websocketUrl && this.welcomeUrl) this.openWebSocket({
            onopen: this.onopen.bind(this),
            onmessage: this.onmessage.bind(this)
        })
    },
    reset () {
        // Remove tags
        for (var name in this.tags) {
            delete this.tags[name]
        }

        // Remove nodes
        for (name in this.nodes) {
            delete this.nodes[name]
        }
    },
    onopen () {
        // Fetch welcome
        this.sendHttp({
            method: 'GET',
            url: this.welcomeUrl,
            onsuccess: function(data) {
                // Reset
                this.reset()

                // No workflow or no nodes
                if (!data.workflow || data.workflow.nodes.length === 0) {
                    // Refresh nodes position
                    this.refreshNodesPosition()
                    return
                }

                // Update workflow name
                this.workflow.name = data.workflow.name

                // Loop through nodes
                data.workflow.nodes.forEach(function(item) {
                    // Apply changes
                    this.apply(item.name, item)
                }.bind(this))

                // Refresh nodes position
                this.refreshNodesPosition()
            }.bind(this)
        })
    },
    onmessage (name, payload, recording) {
        // Do nothing
        if (this.recording.loaded && !recording) return

        // Get rollback
        var rollbacks = [], n = false
        if (recording) {
            switch (name) {
                case 'astiencoder.node.child.added':
                case 'astiencoder.node.child.removed':
                    // Get node
                    n = this.nodes[payload.parent]
                    if (!n) break

                    // Create rollback
                    var rollback = {
                        name: 'astiencoder.node.child.removed',
                        payload: payload
                    }
                    if (name === 'astiencoder.node.child.removed') {
                        rollback.name = 'astiencoder.node.child.added'
                    }

                    // Append rollback
                    rollbacks.push(rollback)
                    break
                case 'astiencoder.node.closed':
                    // Get node
                    n = this.nodes[payload]
                    if (!n) break

                    // Append rollback
                    // "closed" event can be sent several time for the same node
                    if (!n.closed)  rollbacks.push({
                        name: 'astiencoder.rollback.node.unclose',
                        payload: payload
                    })
                    break
                case 'astiencoder.node.continued':
                case 'astiencoder.node.paused':
                case 'astiencoder.node.stopped':
                    // Get node
                    n = this.nodes[payload]
                    if (!n) break

                    // Create rollback
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

                    // Append rollback
                    rollbacks.push(rollback)
                    break
                case 'astiencoder.node.started':
                    // Append rollback
                    rollbacks.push({
                        name: 'astiencoder.rollback.node.remove',
                        payload: payload.name
                    })
                    break
                case 'astiencoder.stats':    
                    // Create rollback
                    rollback = {
                        name: name,
                        payload: []
                    }
    
                    // Add rollbacks
                    this.addStatsRollback(this.workflow.name, this.workflow.stats, rollbacks, rollback)
                    for (var nn in this.nodes) {
                        this.addStatsRollback(nn, this.nodes[nn].stats, rollbacks, rollback)
                    }

                    // Append rollback
                    if (rollback.payload.length > 0) rollbacks.push(rollback)
                    break
            }
        }

        // Apply
        switch (name) {
            case 'astiencoder.node.child.added':
                // Apply
                this.apply(payload.parent, {children: [payload.child]})

                // Refresh nodes position
                if (!recording) this.refreshNodesPosition()
                break
            case 'astiencoder.node.child.removed':
                // Apply
                this.apply(payload.parent, {childrenRemoved: [payload.child]})

                // Refresh nodes position
                if (!recording) this.refreshNodesPosition()
                break
            case 'astiencoder.node.closed':
                // Apply
                this.apply(payload, {closed: true})

                // Refresh nodes position
                if (!recording) this.refreshNodesPosition()
                break
            case 'astiencoder.node.continued':
                this.apply(payload, {status: 'running'})
                break
            case 'astiencoder.node.paused':
                this.apply(payload, {status: 'paused'})
                break
            case 'astiencoder.node.started':
                // Apply
                this.apply(payload.name, payload)

                // Refresh nodes position
                if (!recording) this.refreshNodesPosition()
                break
            case 'astiencoder.node.stopped':
                // Apply
                this.apply(payload, {status: 'stopped'})

                // Refresh nodes position
                if (!recording) this.refreshNodesPosition()
                break
            case 'astiencoder.stats':
                // Apply
                payload.forEach(stat => {
                    this.apply(stat.target, {stat: stat})
                })

                // Refresh nodes size
                if (!recording) this.refreshNodesSize()
                break
        }
        return rollbacks
    },
    addStatsRollback (name, stats, rollbacks, rollback) {
        // No stats
        if (Object.keys(stats).length === 0) {
            // Append rollback
            rollbacks.push({
                name: 'astiencoder.rollback.stats.remove',
                payload: name
            })
            return
        }

        // Loop through stats
        for (var stat in stats) {
            var s = {}
            for (var k in stats[stat]) {
                s[k] = stats[stat][k]
            }
            s.target = name
            rollback.payload.push(s)
        }
    },
    onKeyDown (event) {
        // Keys don't exist
        if (!this.keys) this.keys = {}

        // Add key
        this.keys[event.code] = true
    },
    onKeyUp (event) {
        // Keys don't exist
        if (!this.keys) return

        // Action
        if (this.exactKeys('ArrowDown', 'ArrowUp')) this.zoomAuto()
        else if (this.exactKeys('ArrowDown')) this.zoomOut()
        else if (this.exactKeys('ArrowLeft')) this.onRecordingPreviousClick()
        else if (this.exactKeys('ArrowRight')) this.onRecordingNextClick()
        else if (this.exactKeys('ArrowUp')) this.zoomIn()

        // Remove key
        this.keys[event.code] = false 
    },
    exactKeys(...keys) {
        // Get number of match
        var count = 0
        for (var idx = 0; idx < keys.length; idx++) {
            if (this.keys[keys[idx]]) count++
        }

        // All keys match
        if (count === keys.length) {
            for (idx = 0; idx < keys.length; idx++) {
                this.keys[keys[idx]] = false
            }
            return true
        }
        return false
    },

    /* workflow */
    workflow: {
        name: '',
        stats: new Proxy({}, {
            deleteProperty: function(obj, prop) {
                // Stat doesn't exists
                if (typeof obj[prop] === 'undefined') return

                // Switch on prop
                switch (prop) {
                    case 'astiencoder.host.usage':
                        document.getElementById("memory-virtual").innerText = ''
                        document.getElementById("memory-resident").innerText = ''
                        document.getElementById("memory-used").innerText = ''
                        document.getElementById("memory-total").innerText = ''
                        document.getElementById("cpu-process").innerText = ''
                        document.getElementById("cpu-total").innerText = ''
                        document.getElementById("cpus").innerText = ''
                        break
                }

                // Delete prop
                delete(obj[prop])
            },
            set: function(obj, prop, value) {
                // Nothing changed
                if (typeof obj[prop] !== 'undefined' && obj[prop] === value) return
    
                // Switch on prop
                switch (prop) {
                    case 'astiencoder.host.usage':
                        if (value.value.memory.resident) document.getElementById("memory-resident").innerText = (value.value.memory.resident/Math.pow(1024, 3)).toFixed(2)
                        if (value.value.memory.virtual) document.getElementById("memory-virtual").innerText = (value.value.memory.virtual/Math.pow(1024, 3)).toFixed(2)
                        if (value.value.memory.used) document.getElementById("memory-used").innerText = (value.value.memory.used/Math.pow(1024, 3)).toFixed(2)
                        if (value.value.memory.total) document.getElementById("memory-total").innerText = (value.value.memory.total/Math.pow(1024, 3)).toFixed(2)
                        if (value.value.cpu.process) document.getElementById("cpu-process").innerText = value.value.cpu.process.toFixed(2)
                        if (value.value.cpu.total) document.getElementById("cpu-total").innerText = value.value.cpu.total.toFixed(2)
                        if (value.value.cpu.individual) {
                            var e = document.getElementById("cpus")
                            e.innerHTML = ""
                            for (var idx = 0; idx < value.value.cpu.individual.length; idx++) {
                                e.innerHTML += "<div>#" + (idx + 1) + ": "+ value.value.cpu.individual[idx].toFixed(2) + "%</div>"
                            }
                        }
                        break
                }
    
                // Store value
                obj[prop] = value
                return true
            }
        })
    },

    /* recording */
    recording: new Proxy({
        cursorNexts: 0,
        cursorPreviouses: 0,
        nexts: [],
        previouses: [],
        reset () {
            this.cursorNexts = 0
            this.cursorPreviouses = 0
            this.nexts = []
            this.previouses = []
        },
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
                    case 'astiencoder.rollback.node.unclose':
                        // Get item
                        var i = astiencoder.nodes[item.payload]
                        if (item.payload === astiencoder.workflow.name) i = astiencoder.workflow
                        if (!i) break

                        // Unclose
                        i.closed = false
                        break
                    case 'astiencoder.rollback.node.remove':
                        // Delete
                        delete astiencoder.nodes[item.payload]
                        break
                    case 'astiencoder.rollback.stats.remove':
                        // Get item
                        var i = astiencoder.nodes[item.payload]
                        if (item.payload === astiencoder.workflow.name) i = astiencoder.workflow
                        if (!i) break

                        // Loop through stats
                        for (var stat in i.stats) {
                            delete i.stats[stat]
                        }
                        break
                    default:
                        const rs = astiencoder.onmessage(item.name, item.payload, true)
                        rollbacks.push(...rs)
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

            // Update workflow name
            this.workflow.name = n.payload.name

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
                    case 'astiencoder.node.child.added':
                    case 'astiencoder.node.child.removed':
                        k = n.name + ' | ' + n.payload.parent + ' | ' + n.payload.child
                        break
                    case 'astiencoder.node.closed':
                        k = 'closed | ' + n.payload
                        break
                    case 'astiencoder.node.continued':
                    case 'astiencoder.node.paused':
                    case 'astiencoder.node.stopped':
                        k = 'status | ' + n.payload
                        break
                    case 'astiencoder.node.started':
                        k = 'status | ' + n.payload.name
                        break
                    case 'astiencoder.stats':
                        k = 'stats'
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

        // Reset recording
        this.recording.reset()

        // On open
        this.onopen()
    },
    onRecordingNextClick () {
        // No recording
        if (!this.recording.loaded) return

        // Apply next
        this.recording.applyNext()

        // Refresh nodes position
        this.refreshNodesPosition()
    },
    onRecordingPreviousClick () {
        // No recording
        if (!this.recording.loaded) return

        // Apply previous
        this.recording.applyPrevious()

        // Refresh nodes position
        this.refreshNodesPosition()
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

        // Refresh nodes position
        this.refreshNodesPosition()
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
        set: function(obj, prop) {
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
        // Loop through nodes
        for (var name in this.nodes) {
            this.refreshTagsForNode(name)
        }

        // Refresh nodes position
        this.refreshNodesPosition()
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
        for (tag in this.nodes[name].tags) {
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
        set: function(obj, prop, value) {
            // Node already exists
            if (typeof obj[prop] !== 'undefined') return

            // Create node
            var n = {
                _key: value.label,
                dom: {},
                notInSearch: false,
                notInTags: false
            }

            // We need to store locally the node name since it's used by refreshTagsForNode
            const nodeName = prop

            // Create wrapper
            n.dom.w = document.createElement('div')
            n.dom.w.className = 'node'

            // Add children
            n.children = new Proxy({}, {
                set: function(obj, prop) {
                    // Nothing changed
                    if (typeof obj[prop] !== 'undefined') return

                    // Store value
                    obj[prop] = {
                        arrow: {
                            head: {
                                line1: document.createElementNS("http://www.w3.org/2000/svg", "line"),
                                line2: document.createElementNS("http://www.w3.org/2000/svg", "line")
                            },
                            line: document.createElementNS("http://www.w3.org/2000/svg", "line")
                        }
                    }
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
            n.parents = {}

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

            // Methods
            n.displayed = function() {
                return !this.notInTags && !this.notInSearch
            }

            // Store node
            obj[prop] = new Proxy(n, {
                set: function(obj, prop, value) {
                    // Nothing changed
                    if (typeof obj[prop] !== 'undefined' && obj[prop] === value) return

                    // Switch on prop
                    var refreshTags = false
                    switch (prop) {
                        case 'closed':
                            if (obj[prop] !== value) {
                                // Update tags
                                delete(n.tags[prop])
                                if (value) n.tags[prop] = true

                                // Make sure to refresh tags
                                refreshTags = true
                            }
                            break
                        case 'label':
                            _l.innerText = value
                            break
                        case 'name':
                            _n.innerText = value
                            break
                        case 'status':
                            if (obj[prop] !== value) {
                                // Update class
                                n.dom.w.classList.remove(obj[prop])
                                n.dom.w.classList.add(value)

                                // Update tags
                                delete(n.tags[obj[prop]])
                                n.tags[value] = true

                                // Make sure to refresh tags
                                refreshTags = true
                            }
                            break
                    }

                    // Refresh tags
                    if (refreshTags) astiencoder.refreshTagsForNode(nodeName)

                    // Store value
                    obj[prop] = value
                    return true
                }
            })
            return true
        }
    }),
    apply (name, payload) {
        if (this.workflow.name === name) this.applyToWorkflow(payload)
        else this.applyToNode(name, payload)
    },
    applyToWorkflow (payload) {
        // Stat
        if (payload.stat) {
            this.workflow.stats[payload.stat.name] = payload.stat
        }
    },
    applyToNode (name, payload) {
        // Add node
        // If node already exists, it will do nothing
        this.nodes[name] = payload

        // Children
        if (payload.children) {
            payload.children.forEach(function(item) {
                // Update node
                this.nodes[name].children[item] = true

                // Update child
                const c = this.nodes[item]
                if (c) c.parents[name] = true
            }.bind(this))
        }

        // Removed children
        if (payload.childrenRemoved) {
            payload.childrenRemoved.forEach(function(item) {
                // Update node
                delete this.nodes[name].children[item]

                // Update child
                const c = this.nodes[item]
                if (c) delete c.parents[name]
            }.bind(this))
        }

        // Closed
        if (typeof payload.closed !== 'undefined') this.nodes[name].closed = payload.closed

        // Description
        if (payload.description) this.nodes[name].description = payload.description

        // Label
        if (payload.label) this.nodes[name].label = payload.label

        // Name
        if (payload.name) this.nodes[name].name = payload.name

        // Parents
        if (payload.parents) {
            payload.parents.forEach(function(item) {
                // Update node
                this.nodes[name].parents[item] = true

                // Update parent
                const p = this.nodes[item]
                if (p) p.children[name] = true
            }.bind(this))
        }

        // Stat
        if (payload.stat) {
            // Add stat
            // If stat already exists, it will do nothing
            this.nodes[name].stats[payload.stat.label] = payload.stat

            // Description
            if (payload.stat.description) this.nodes[name].stats[payload.stat.label].description = payload.stat.description

            // Label
            if (payload.stat.label) this.nodes[name].stats[payload.stat.label].label = payload.stat.label

            // Unit
            // Process unit before value since we need the updated unit when handling the value
            if (payload.stat.unit) this.nodes[name].stats[payload.stat.label].unit = payload.stat.unit

            // Value
            if (typeof payload.stat.value !== 'undefined') {
                var v = payload.stat.value
                var f = parseFloat(v)
                if (!isNaN(f)) {
                    switch (this.nodes[name].stats[payload.stat.label].unit) {
                        case 'bps':
                            if (f > 1e9) {
                                f /= 1e9
                                this.nodes[name].stats[payload.stat.label].unit = 'Gbps'
                            } else if (f > 1e6) {
                                f /= 1e6
                                this.nodes[name].stats[payload.stat.label].unit = 'Mbps'
                            } else if (f > 1e3) {
                                f /= 1e3
                                this.nodes[name].stats[payload.stat.label].unit = 'kbps'
                            }
                            break
                        case 'ns':
                            if (f > 1e9 || f < -1e9) {
                                f /= 1e9
                                this.nodes[name].stats[payload.stat.label].unit = 's'
                            } else if (f > 1e6 || f < -1e6) {
                                f /= 1e6
                                this.nodes[name].stats[payload.stat.label].unit = 'ms'
                            } else if (f > 1e3 || f < -1e3) {
                                f /= 1e3
                                this.nodes[name].stats[payload.stat.label].unit = 'µs'
                            }
                            break
                    }
                    f = f.toFixed(2)
                    if (f < 10 && f >= 0) f = '0' + f
                    else if (f > -10 && f < 0) f = '-0' + (-f)
                    if (this.nodes[name].stats[payload.stat.label].unit === '%') {
                        if (f >= 1000) f = '+∞'
                        else if (f <= -1000) f = '-∞'
                    }
                }
                this.nodes[name].stats[payload.stat.label].value = f
            }
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

    /* nodes position refresher */
    initNodesPositionRefresher () {
        this.nodesPositionRefresher = {
            a: new Uint8Array(1),
            f: function() {
                // Reset
                document.getElementById('nodes').innerHTML = ''

                // Create svg
                const svg = document.createElementNS("http://www.w3.org/2000/svg", "svg")
                document.getElementById('nodes').appendChild(svg)

                // Get levels
                var processedNodes = {}, total = Object.keys(this.nodes).length, levels = []
                while (Object.keys(processedNodes).length < total) {
                    // Loop through nodes
                    var level = [], tempLevel = {}
                    for (var name in this.nodes) {
                        // Already processed
                        if (processedNodes[name]) continue

                        // There are no levels yet, we check nodes with no parents
                        if (levels.length === 0) {
                            if (Object.keys(this.nodes[name].parents).length === 0) level.push(this.nodes[name])
                            continue
                        }
                        
                        // Loop through previous level nodes
                        levels[levels.length-1].forEach(item => {
                            if (this.nodes[name].parents[item.name]) tempLevel[name] = this.nodes[name]
                        })
                    }

                    // Move children in the same zone as their parent
                    if (levels.length > 0 && Object.keys(tempLevel).length > 0) {
                        // Loop through previous level nodes
                        levels[levels.length - 1].forEach(item => {
                            // Loop through children
                            for (var k in item.children) {
                                const c = tempLevel[k]
                                if (c) {
                                    level.push(c)
                                    delete tempLevel[k]
                                }
                            }
                        })
                    }

                    // No nodes in level
                    // This shouldn't happen but we want to avoid infinite loops
                    if (level.length === 0) break

                    // Append level
                    levels.push(level)
                    level.forEach(node => processedNodes[node.name] = true)
                }

                // Loop through levels
                levels.forEach(level => {
                    // Create wrapper
                    const lw = document.createElement('div')

                    // Loop through level items
                    level.forEach(item => {
                        // Node is not displayed
                        if (!item.displayed()) return

                        // Append wrapper
                        lw.appendChild(item.dom.w)

                        // Loop through children
                        for (var c in item.children) {
                            // Node is not displayed
                            const n = this.nodes[c]
                            if (!n || !n.displayed()) continue

                            // Append arrow
                            svg.appendChild(item.children[c].arrow.line)
                            svg.appendChild(item.children[c].arrow.head.line1)
                            svg.appendChild(item.children[c].arrow.head.line2)
                        }
                    })

                    // Append wrapper
                    document.getElementById('nodes').appendChild(lw)
                })
                
                // Refresh size
                this.refreshNodesSize()
            }.bind(this),
            i: setInterval(function() {
                // Nothing to do
                if (Atomics.compareExchange(this.nodesPositionRefresher.a, 0, 1, 0) === 0) {
                    return
                }

                // Refresh nodes position
                this.nodesPositionRefresher.f()
            }.bind(this), 50)
        }
    },
    refreshNodesPosition () {
        Atomics.store(this.nodesPositionRefresher.a, 0, 1)
    },
    refreshNodesSize () {
        // Get zoom ratio
        var zoomRatio = this.zoom.value / 100
        if (this.zoom.auto) {
            // Get total width
            var { totalWidth } = this.nodesTotalSize(1)

            // Update zoom ratio
            zoomRatio = Math.min(document.querySelector('section').offsetWidth / totalWidth, 1)

            // Update value
            this.zoom.value = zoomRatio * 100
        }

        // Get total size
        var { levelHeights, levelWidths, totalHeight, totalWidth } = this.nodesTotalSize(zoomRatio)

        // Get levels
        const ls = document.querySelectorAll('#nodes > div')

        // Loop through levels
        var cursorHeight = 0
        for (var idxLevel = 0; idxLevel < ls.length; idxLevel++) {
            // Get level size
            const levelHeight = levelHeights[idxLevel]
            const levelWidth = levelWidths[idxLevel]

            // Get horizontal margin between nodes
            const horizontalMargin = (totalWidth - levelWidth) / ls[idxLevel].childNodes.length / 2

            // Loop through nodes
            var cursorWidth = 0
            for (var idxItem = 0; idxItem < ls[idxLevel].childNodes.length; idxItem++) {
                // Update top
                ls[idxLevel].childNodes[idxItem].style.top = cursorHeight + ((levelHeight - ls[idxLevel].childNodes[idxItem].offsetHeight) / 2) + 'px'

                // Update left
                if (idxItem === 0) ls[idxLevel].childNodes[idxItem].style.left = horizontalMargin + 'px'
                else ls[idxLevel].childNodes[idxItem].style.left = cursorWidth + 2 * horizontalMargin + 'px'

                // Update width cursor
                cursorWidth = ls[idxLevel].childNodes[idxItem].offsetLeft + ls[idxLevel].childNodes[idxItem].offsetWidth - 1
            }

            // Update height cursor
            if (levelHeight > 0) cursorHeight += levelHeight + this.nodeBottomMargin(zoomRatio)
        }

        // Loop through nodes
        for (var name in this.nodes) {
            // Node is not displayed
            const n = this.nodes[name]
            if (!n.displayed()) continue
            
            // Loop through children
            for (var k in this.nodes[name].children) {
                // Node is not displayed
                const c = this.nodes[k]
                if (!c || !c.displayed()) continue

                // Node has no height
                if (c.dom.w.offsetHeight === 0) continue

                // Get direction
                var d = ""
                if ((n.dom.w.offsetTop <= c.dom.w.offsetTop && n.dom.w.offsetTop + n.dom.w.offsetHeight >= c.dom.w.offsetTop + c.dom.w.offsetHeight) || (c.dom.w.offsetTop <= n.dom.w.offsetTop && c.dom.w.offsetTop + c.dom.w.offsetHeight >= n.dom.w.offsetTop + c.dom.w.offsetHeight)) {
                    d+= "center-"
                } else if (n.dom.w.offsetTop + n.dom.w.offsetHeight < c.dom.w.offsetTop) d += "bottom-"
                else d += "top-"
                if (d === "bottom-" || d === "top-") {
                    if (n.dom.w.offsetLeft + (n.dom.w.offsetWidth / 2) < c.dom.w.offsetLeft + (c.dom.w.offsetWidth / 2)) d += "right"
                    else d += "left"
                } else {
                    if (n.dom.w.offsetLeft + n.dom.w.offsetWidth < c.dom.w.offsetLeft) d += "right"
                    else d += "left"
                }

                // Get arrow line coordinates
                var lineFromX, lineFromY, lineToX, lineToY
                if (d.startsWith("bottom-")) {
                    lineFromX = n.dom.w.offsetLeft + (n.dom.w.offsetWidth / 2)
                    lineFromY = n.dom.w.offsetTop + n.dom.w.offsetHeight
                    lineToX = c.dom.w.offsetLeft + (c.dom.w.offsetWidth / 2)
                    lineToY = c.dom.w.offsetTop
                } else if (d.startsWith("top-")) {
                    lineFromX = n.dom.w.offsetLeft + (n.dom.w.offsetWidth / 2)
                    lineFromY = n.dom.w.offsetTop
                    lineToX = c.dom.w.offsetLeft + (c.dom.w.offsetWidth / 2)
                    lineToY = c.dom.w.offsetTop + c.dom.w.offsetHeight
                } else if ((d.endsWith("-right"))) {
                    lineFromX = n.dom.w.offsetLeft + n.dom.w.offsetWidth
                    lineFromY = n.dom.w.offsetTop + (n.dom.w.offsetHeight / 2)
                    lineToX = c.dom.w.offsetLeft
                    lineToY = c.dom.w.offsetTop + (c.dom.w.offsetHeight / 2)
                } else {
                    lineFromX = n.dom.w.offsetLeft
                    lineFromY = n.dom.w.offsetTop + (n.dom.w.offsetHeight / 2)
                    lineToX = c.dom.w.offsetLeft + c.dom.w.offsetWidth
                    lineToY = c.dom.w.offsetTop + (c.dom.w.offsetHeight / 2)
                }

                // Update arrow line
                this.nodes[name].children[k].arrow.line.style.strokeWidth = zoomRatio + 'px'
                this.nodes[name].children[k].arrow.line.setAttribute('x1', lineFromX)
                this.nodes[name].children[k].arrow.line.setAttribute('y1', lineFromY)
                this.nodes[name].children[k].arrow.line.setAttribute('x2', lineToX)
                this.nodes[name].children[k].arrow.line.setAttribute('y2', lineToY)

                // Get angles
                const lineAngle = Math.atan(Math.abs(lineToX - lineFromX) / Math.abs(lineToY - lineFromY))
                const oppositeLineAngle = (Math.PI / 2 - lineAngle)
                const arrowAngle = 30 * Math.PI / 180

                // Get arrow length
                const arrowLength = this.nodeArrowLength(zoomRatio)

                // Get arrow head line coordinates
                var headLine1FromX, headLine1ToX, headLine1FromY, headLine1ToY, headLine2FromX, headLine2ToX, headLine2FromY, headLine2ToY
                if (d.endsWith("-right")) {
                    headLine1FromX = lineToX - (Math.sin(lineAngle - arrowAngle) * 0.5)
                    headLine1ToX = headLine1FromX - (Math.sin(lineAngle - arrowAngle) * arrowLength)
                    headLine2FromX = lineToX - (Math.cos(oppositeLineAngle - arrowAngle) * 0.5)
                    headLine2ToX = headLine2FromX - (Math.cos(oppositeLineAngle - arrowAngle) * arrowLength)
                } else {
                    headLine1FromX = lineToX + (Math.sin(lineAngle - arrowAngle) * 0.5)
                    headLine1ToX = headLine1FromX + (Math.sin(lineAngle - arrowAngle) * arrowLength)
                    headLine2FromX = lineToX + (Math.cos(oppositeLineAngle - arrowAngle) * 0.5)
                    headLine2ToX = headLine2FromX + (Math.cos(oppositeLineAngle - arrowAngle) * arrowLength)
                }
                if (d.startsWith("bottom-")) {
                    headLine1FromY = lineToY - (Math.cos(lineAngle - arrowAngle) * 0.5)
                    headLine1ToY = headLine1FromY - (Math.cos(lineAngle - arrowAngle) * arrowLength)
                    headLine2FromY = lineToY - (Math.sin(oppositeLineAngle - arrowAngle) * 0.5)
                    headLine2ToY = headLine2FromY - (Math.sin(oppositeLineAngle - arrowAngle) * arrowLength)
                } else {
                    headLine1FromY = lineToY + (Math.cos(lineAngle - arrowAngle) * 0.5)
                    headLine1ToY = headLine1FromY + (Math.cos(lineAngle - arrowAngle) * arrowLength)
                    headLine2FromY = lineToY + (Math.sin(oppositeLineAngle - arrowAngle) * 0.5)
                    headLine2ToY = headLine2FromY + (Math.sin(oppositeLineAngle - arrowAngle) * arrowLength)
                }

                // Update arrow head lines
                this.nodes[name].children[k].arrow.head.line1.style.strokeWidth = zoomRatio + 'px'
                this.nodes[name].children[k].arrow.head.line1.setAttribute('x1', headLine1FromX)
                this.nodes[name].children[k].arrow.head.line1.setAttribute('y1', headLine1FromY)
                this.nodes[name].children[k].arrow.head.line1.setAttribute('x2', headLine1ToX)
                this.nodes[name].children[k].arrow.head.line1.setAttribute('y2', headLine1ToY)
                this.nodes[name].children[k].arrow.head.line2.style.strokeWidth = zoomRatio + 'px'
                this.nodes[name].children[k].arrow.head.line2.setAttribute('x1', headLine2FromX)
                this.nodes[name].children[k].arrow.head.line2.setAttribute('y1', headLine2FromY)
                this.nodes[name].children[k].arrow.head.line2.setAttribute('x2', headLine2ToX)
                this.nodes[name].children[k].arrow.head.line2.setAttribute('y2', headLine2ToY)
            }
        }

        // Update size
        document.getElementById('nodes').style.height = totalHeight + 'px'
        document.getElementById('nodes').style.width = totalWidth + 'px'
    },
    nodeLeftMargin (zoomRatio) {
        return 2 * 12 * zoomRatio
    },
    nodeBottomMargin (zoomRatio) {
        return 4 * 12 * zoomRatio
    },
    nodeArrowLength (zoomRatio) {
        return 12 * zoomRatio
    },
    nodesTotalSize (zoomRatio) {
        // Reset
        document.getElementById('nodes').style.fontSize = 12 * zoomRatio + 'px'
        document.getElementById('nodes').style.height = 1e10 + 'px'
        document.getElementById('nodes').style.width = 1e10 + 'px'

        // Get levels
        const ls = document.querySelectorAll('#nodes > div')

        // Loop through levels
        var totalWidth = 0, levelWidths = [], levelHeights = [], totalHeight = this.nodeBottomMargin(zoomRatio) * (ls.length - 1)
        for (var idxLevel = 0; idxLevel < ls.length; idxLevel++) {
            // Create sizes
            var height = 0
            var width = 0

            // Update width
            ls[idxLevel].childNodes.forEach(item => {
                height = Math.max(height, item.getBoundingClientRect().height)
                width += item.getBoundingClientRect().width
            })

            // Update
            levelHeights.push(height)
            levelWidths.push(width)
            totalHeight += height
            totalWidth = Math.max(totalWidth, width + this.nodeLeftMargin(zoomRatio) * ls[idxLevel].childNodes.length)
        }
        return {
            levelHeights,
            levelWidths,
            totalHeight,
            totalWidth
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

        // Refresh nodes position
        this.refreshNodesPosition()
    },

    /* zoom */
    zoom: {
        auto: true,
        value: 100
    },
    zoomIn () {
        // No nodes
        if (Object.keys(this.nodes).length === 0) return

        // Update zoom
        this.zoom.auto = false
        this.zoom.value += 10

        // Refresh nodes size
        this.refreshNodesSize()
    },
    zoomOut () {
        // No nodes
        if (Object.keys(this.nodes).length === 0) return

        // Invalid value
        if (this.zoom.value <= 10) return

        // Update zoom
        this.zoom.auto = false
        this.zoom.value -= 10

        // Refresh nodes size
        this.refreshNodesSize()
    },
    zoomAuto () {
        // No nodes
        if (Object.keys(this.nodes).length === 0) return
        
        // Update zoom
        this.zoom.auto = true

        // Refresh nodes size
        this.refreshNodesSize()
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

        // Create websocket
        this.ws = new WebSocket(this.websocketUrl)
    
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
            options.onmessage(data.event_name, data.payload, false)
        }.bind(this)
    },
    sendWebSocket (name, payload) {
        if (!this.ws) return
        var d = {event_name: name}
        if (payload) d.payload = payload
        this.ws.send(JSON.stringify(d))
    }
}

astiencoder.init()
