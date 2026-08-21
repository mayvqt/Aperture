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

document.querySelectorAll(".workflow-card").forEach(function (card) {
    var summary = card.querySelector("summary");
    if (!summary) return;

    summary.addEventListener("click", function (event) {
        event.preventDefault();
        if (card.dataset.animating === "true") return;

        if (card.open) {
            animateWorkflow(card, false);
            return;
        }

        var group = card.closest(".template-workflows");
        if (group) {
            group.querySelectorAll(".workflow-card[open]").forEach(function (other) {
                if (other !== card) animateWorkflow(other, false);
            });
        }
        animateWorkflow(card, true);
    });
});

function animateWorkflow(card, opening) {
    if (window.matchMedia("(prefers-reduced-motion: reduce)").matches) {
        card.open = opening;
        return;
    }

    var startHeight = card.offsetHeight;
    if (opening) card.open = true;
    var endHeight = opening ? card.scrollHeight : card.querySelector("summary").offsetHeight;

    card.dataset.animating = "true";
    card.style.overflow = "hidden";
    var animation = card.animate(
        {height: [startHeight + "px", endHeight + "px"]},
        {duration: 220, easing: "cubic-bezier(.2, .8, .2, 1)"}
    );

    animation.onfinish = function () {
        card.open = opening;
        card.style.height = "";
        card.style.overflow = "";
        delete card.dataset.animating;
    };

    animation.oncancel = animation.onfinish;
}
