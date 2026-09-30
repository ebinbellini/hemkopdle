function request_double_frame(callback) {
    requestAnimationFrame(() => {
        requestAnimationFrame(callback);
    });
}

function open_mode_picker() {
    const picker = document.getElementById("mode-picker")
    picker.style.display = "block";
    request_double_frame(() => {
        picker.classList.add("open");
    });
}

function close_mode_picker() {
    const picker = document.getElementById("mode-picker")
    picker.classList.remove("open");
    setTimeout(() => {
        picker.style.removeProperty("display");
    }, 200);
}