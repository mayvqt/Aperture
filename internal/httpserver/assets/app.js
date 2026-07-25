document.addEventListener("click", function (event) {
  var button = event.target.closest("[data-copy]");
  if (!button || !navigator.clipboard) return;

  var target = document.getElementById(button.getAttribute("data-copy"));
  if (!target) return;

  navigator.clipboard.writeText(target.textContent.trim()).then(function () {
    var old = button.textContent;
    button.textContent = "Copied";
    window.setTimeout(function () { button.textContent = old; }, 1600);
  });
});

document.addEventListener("submit", function (event) {
  var form = event.target.closest("[data-confirm]");
  if (form && !window.confirm(form.getAttribute("data-confirm"))) {
    event.preventDefault();
  }
});
