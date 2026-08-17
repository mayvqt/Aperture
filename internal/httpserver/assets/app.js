document.addEventListener("click", function (event) {
    var button = event.target.closest("[data-copy]");
    if (!button || !navigator.clipboard) return;

    var target = document.getElementById(button.getAttribute("data-copy"));
    if (!target) return;

    navigator.clipboard.writeText(target.textContent.trim()).then(function () {
        var old = button.textContent;
        button.textContent = "Copied";
        window.setTimeout(function () {
            button.textContent = old;
        }, 1600);
    });
});

document.addEventListener("submit", function (event) {
    var form = event.target.closest("[data-confirm]");
    if (form && !window.confirm(form.getAttribute("data-confirm"))) {
        event.preventDefault();
    }
});

(function () {
    var button = document.querySelector(".nav-more-button");
    var menu = document.getElementById("admin-more-menu");
    if (!button || !menu) return;

    function closeMenu() {
        button.setAttribute("aria-expanded", "false");
        menu.hidden = true;
    }

    button.addEventListener("click", function () {
        var opening = button.getAttribute("aria-expanded") !== "true";
        button.setAttribute("aria-expanded", String(opening));
        menu.hidden = !opening;
    });

    document.addEventListener("click", function (event) {
        if (!event.target.closest(".nav-more")) closeMenu();
    });

    document.addEventListener("keydown", function (event) {
        if (event.key === "Escape" && !menu.hidden) {
            closeMenu();
            button.focus();
        }
    });
})();
