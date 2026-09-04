document.addEventListener("click", function (event) {
    var button = event.target.closest("[data-copy]");
    if (!button) return;

    var status = document.getElementById(button.getAttribute("aria-describedby"));
    function report(message) {
        if (status) status.textContent = message;
    }

    if (!navigator.clipboard || !navigator.clipboard.writeText) {
        report("Copy is unavailable. Select the URL and copy it manually.");
        return;
    }

    var target = document.getElementById(button.getAttribute("data-copy"));
    if (!target) {
        report("Could not find the invite URL. Select and copy it manually.");
        return;
    }

    navigator.clipboard.writeText(target.textContent.trim()).then(function () {
        var old = button.textContent;
        button.textContent = "Copied";
        report("Invite URL copied.");
        window.setTimeout(function () {
            button.textContent = old;
        }, 1600);
    }, function () {
        report("Copy failed. Select the URL and copy it manually.");
    });
});

function syncCustomExpiry(select) {
    var field = document.getElementById(select.getAttribute("aria-controls"));
    if (!field) return;
    var input = field.querySelector("input[name=expires_at]");
    var custom = select.value === "custom";
    field.hidden = !custom;
    if (input) input.disabled = !custom;
}

document.querySelectorAll("select[name=expires_after_days][aria-controls]").forEach(function (select) {
    syncCustomExpiry(select);
    select.addEventListener("change", function () {
        syncCustomExpiry(select);
    });
});

document.addEventListener("submit", function (event) {
    var form = event.target.closest("[data-confirm]");
    if (form && !window.confirm(form.getAttribute("data-confirm"))) {
        event.preventDefault();
    }
});

var adminNav = document.querySelector("nav.admin-nav");
if (adminNav) {
    var currentNavItem = adminNav.querySelector('a[aria-current="page"]');
    var reducedMotion = window.matchMedia("(prefers-reduced-motion: reduce)");
    if (currentNavItem && adminNav.scrollWidth > adminNav.clientWidth) {
        currentNavItem.scrollIntoView({
            block: "nearest",
            inline: "nearest",
            behavior: reducedMotion.matches ? "auto" : "smooth"
        });
    }
}

var animatingWorkflows = new WeakSet();
var reduceMotion = window.matchMedia("(prefers-reduced-motion: reduce)");

document.addEventListener("click", function (event) {
    var summary = event.target.closest(".workflow-card > summary");
    if (!summary) return;

    var card = summary.parentElement;
    if (typeof card.animate !== "function" || reduceMotion.matches) return;

    event.preventDefault();
    if (animatingWorkflows.has(card)) return;

    var opening = !card.open;
    if (opening) {
        var group = card.closest(".workflow-group");
        if (group) {
            group.querySelectorAll(".workflow-card[open]").forEach(function (other) {
                if (other !== card) animateWorkflow(other, false);
            });
        }
    }
    animateWorkflow(card, opening);
});

function animateWorkflow(card, opening) {
    if (animatingWorkflows.has(card)) return;

    var startHeight = card.offsetHeight;
    if (opening) card.open = true;
    var endHeight = opening ? card.scrollHeight : card.querySelector("summary").offsetHeight;

    animatingWorkflows.add(card);
    card.style.overflow = "hidden";
    var animation = card.animate(
        {height: [startHeight + "px", endHeight + "px"]},
        {duration: 220, easing: "cubic-bezier(.2, .8, .2, 1)"}
    );

    animation.onfinish = function () {
        card.open = opening;
        card.style.height = "";
        card.style.overflow = "";
        animatingWorkflows.delete(card);
    };
}
