function Modal() {
    this.wrapper = document.createElement("div")
    this.wrapper.className = "modal-wrapper"
}

Modal.prototype.addError = function() {
    this.error = document.createElement("div")
    this.error.className = "modal-error color-danger-text"
    this.wrapper.appendChild(this.error)
}

Modal.prototype.setError = function(error) {
    this.error.innerText = error
    this.error.style.display = "block"
}

Modal.prototype.resetError = function() {
    this.error.style.display = "none"
}

Modal.prototype.addTitle = function(title) {
    const el = document.createElement("div")
    el.innerText = title
    el.className = "modal-title"
    this.wrapper.appendChild(el)
}

Modal.prototype.addLabel = function(label) {
    const el = document.createElement("div")
    el.className = "modal-label"
    el.innerText = label
    this.wrapper.appendChild(el)
}

Modal.prototype.addInputText = function() {
    const el = document.createElement("input")
    el.className = "modal-input"
    el.type = "text"
    this.wrapper.appendChild(el)
    return el
}

Modal.prototype.addInputFile = function() {
    const el = document.createElement("input")
    el.type = "file"
    this.wrapper.appendChild(el)
    return el
}

Modal.prototype.addSubmit = function(label, onclick) {
    const el = document.createElement("button")
    el.innerText = label
    el.className = "modal-submit color-success-front"
    el.onclick = onclick
    this.wrapper.appendChild(el)
}