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
        card.closest(".template-workflows").querySelectorAll(".workflow-card[open]").forEach(function (other) {
            if (other !== card) animateWorkflow(other, false);
        });
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
