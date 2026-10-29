/* ---------------------------------------------------------------- *
 * Navbar                                                           *
 * Shared by every page: the mode picker is a slide-in menu on       *
 * narrow screens and a row of buttons on wide ones.                 *
 * ---------------------------------------------------------------- */

function request_double_frame(callback) {
    requestAnimationFrame(() => {
        requestAnimationFrame(callback);
    });
}

function open_mode_picker() {
    const picker = document.getElementById("mode-picker");
    if (!picker) {
        return;
    }
    picker.style.display = "block";
    request_double_frame(() => {
        picker.classList.add("open");
    });
}

function close_mode_picker() {
    const picker = document.getElementById("mode-picker");
    if (!picker) {
        return;
    }
    picker.classList.remove("open");
    setTimeout(() => {
        picker.style.removeProperty("display");
    }, 200);
}
