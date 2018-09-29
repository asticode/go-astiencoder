if (typeof asticode === "undefined") {
    var asticode = {}
}
asticode.tools = {
    getParameterByName(name, url) {
        if (!url) url = window.location.href
        name = name.replace(/[\[\]]/g, "\\$&")
        var regex = new RegExp("[?&]" + name + "(=([^&#]*)|&|#|$)"),
            results = regex.exec(url)
        if (!results) return null
        if (!results[2]) return ''
        return decodeURIComponent(results[2].replace(/\+/g, " "))
    },
    sendHttp: function(options) {
        const req = new XMLHttpRequest()
        req.onreadystatechange = function() {
            if (this.readyState === XMLHttpRequest.DONE) {
                // Parse data
                let data = {responseText: this.responseText, err: null}
                if (this.responseText.length > 0 && this.getResponseHeader("content-type") === "application/json") {
                    try {
                        data.responseJSON = JSON.parse(this.responseText)
                    } catch (e) {
                        data.err = e
                    }
                }

                // Callbacks
                if (data.err === null && this.status >= 200 && this.status < 300) {
                    if (typeof options.success !== "undefined") options.success(data)
                } else {
                    if (typeof options.error !== "undefined") options.error(data)
                }
            }
        }
        req.open(options.method, options.url, true)
        req.send(options.payload)
    },
    appendSorted: function(rootSelector, data, map) {
        // Find proper key
        let key;
        for (let k in map) {
            if (map.hasOwnProperty(k)) {
                if (map[k].name > data.name && (typeof key === "undefined" || map[key].name > map[k].name)) {
                    key = k;
                    break;
                }
            }
        }

        // Update html
        if (typeof key !== "undefined") {
            rootSelector.insertBefore(data.html.wrapper, map[key].html.wrapper);
        } else {
            rootSelector.append(data.html.wrapper);
        }
    },
    removeClass: function(node, name) {
        // Get class name funcs
        let classNameFuncs = this.classNameFuncs(node)

        // No class name funcs
        if (!classNameFuncs) return

        // Remove
        let names = classNameFuncs[0]().split(" ")
        for (let idx = 0; idx < names.length; idx++) {
            if (names[idx] === name) {
                names.splice(idx, 1)
                idx--
            }
        }

        // Set class name
        classNameFuncs[1](names.join(" "))
    },
    addClass: function(node, name) {
        // Get class name funcs
        let classNameFuncs = this.classNameFuncs(node)

        // No class name funcs
        if (!classNameFuncs) return

        // Set class name
        classNameFuncs[1](classNameFuncs[0]() + " " + name)
    },
    classNameFuncs: function(node) {
        switch (typeof node.className) {
            case "string":
                return [
                    function() {
                        return node.className
                    },
                    function(name) {
                        node.className = name
                    },
                ]
            case "object":
                switch (node.className.constructor.name) {
                    case "SVGAnimatedString":
                        return [
                            function() {
                                return node.className.baseVal
                            },
                            function(name) {
                                node.className.baseVal = name
                            }
                        ]
                    default:
                        return false
                }
            default:
                return false
        }
    }
}