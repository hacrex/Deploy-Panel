/* DeployPanel marketing site — small, dependency-free enhancements. */
(function () {
  "use strict";

  var reduceMotion = window.matchMedia && window.matchMedia("(prefers-reduced-motion: reduce)").matches;

  /* ---- mobile navigation ------------------------------------------------- */

  var menuToggle = document.querySelector(".menu-toggle");
  var primaryNav = document.getElementById("primary-nav");

  function setMenu(open) {
    if (!menuToggle || !primaryNav) return;
    primaryNav.classList.toggle("is-open", open);
    menuToggle.setAttribute("aria-expanded", String(open));
    var icon = menuToggle.querySelector("span");
    if (icon) icon.textContent = open ? "−" : "＋";
  }

  if (menuToggle && primaryNav) {
    menuToggle.addEventListener("click", function () {
      setMenu(!primaryNav.classList.contains("is-open"));
    });

    primaryNav.addEventListener("click", function (event) {
      if (event.target.closest("a") && window.matchMedia("(max-width: 1000px)").matches) {
        setMenu(false);
      }
    });

    window.addEventListener("resize", function () {
      if (window.innerWidth > 1000) setMenu(false);
    });
  }

  /* ---- nav dropdowns ----------------------------------------------------- */

  var groups = Array.prototype.slice.call(document.querySelectorAll(".nav-group"));

  function closeGroups(except) {
    groups.forEach(function (group) {
      if (group !== except) group.open = false;
    });
  }

  groups.forEach(function (group) {
    group.addEventListener("toggle", function () {
      if (group.open) closeGroups(group);
    });
  });

  document.addEventListener("click", function (event) {
    if (!event.target.closest(".nav-group")) closeGroups(null);
  });

  document.addEventListener("keydown", function (event) {
    if (event.key !== "Escape") return;
    closeGroups(null);
    if (primaryNav && primaryNav.classList.contains("is-open")) {
      setMenu(false);
      if (menuToggle) menuToggle.focus();
    }
  });

  /* ---- product walkthrough tabs ----------------------------------------- */

  var tabs = Array.prototype.slice.call(document.querySelectorAll('[role="tab"]'));
  var panes = Array.prototype.slice.call(document.querySelectorAll('[role="tabpanel"]'));

  function activateTab(tab) {
    tabs.forEach(function (item) {
      var selected = item === tab;
      item.setAttribute("aria-selected", String(selected));
      item.tabIndex = selected ? 0 : -1;
    });
    panes.forEach(function (pane) {
      pane.hidden = pane.id !== tab.getAttribute("aria-controls");
    });
  }

  tabs.forEach(function (tab, index) {
    tab.addEventListener("click", function () {
      activateTab(tab);
    });

    tab.addEventListener("keydown", function (event) {
      var next = null;
      if (event.key === "ArrowRight") next = tabs[(index + 1) % tabs.length];
      if (event.key === "ArrowLeft") next = tabs[(index - 1 + tabs.length) % tabs.length];
      if (event.key === "Home") next = tabs[0];
      if (event.key === "End") next = tabs[tabs.length - 1];
      if (!next) return;
      event.preventDefault();
      activateTab(next);
      next.focus();
    });
  });

  /* ---- copy the quickstart command -------------------------------------- */

  var copyButton = document.querySelector(".copy-btn");

  function copyText(text) {
    if (navigator.clipboard && window.isSecureContext) {
      return navigator.clipboard.writeText(text);
    }
    return new Promise(function (resolve, reject) {
      var area = document.createElement("textarea");
      area.value = text;
      area.setAttribute("readonly", "");
      area.style.position = "fixed";
      area.style.opacity = "0";
      document.body.appendChild(area);
      area.select();
      try {
        document.execCommand("copy") ? resolve() : reject(new Error("copy failed"));
      } catch (error) {
        reject(error);
      } finally {
        document.body.removeChild(area);
      }
    });
  }

  if (copyButton) {
    copyButton.addEventListener("click", function () {
      var target = document.getElementById(copyButton.getAttribute("data-copy-target"));
      if (!target) return;
      var text = target.textContent.replace(/\n{3,}/g, "\n\n").trim() + "\n";

      copyText(text).then(function () {
        copyButton.textContent = "Copied";
        copyButton.classList.add("copied");
        window.setTimeout(function () {
          copyButton.textContent = "Copy";
          copyButton.classList.remove("copied");
        }, 1800);
      }).catch(function () {
        var range = document.createRange();
        range.selectNodeContents(target);
        var selection = window.getSelection();
        if (selection) {
          selection.removeAllRanges();
          selection.addRange(range);
        }
        copyButton.textContent = "Press Ctrl+C";
        window.setTimeout(function () {
          copyButton.textContent = "Copy";
          if (selection) selection.removeAllRanges();
        }, 2600);
      });
    });
  }

  /* ---- header shadow ----------------------------------------------------- */

  var header = document.querySelector(".site-header");
  var ticking = false;

  function onScroll() {
    if (!header || ticking) return;
    ticking = true;
    window.requestAnimationFrame(function () {
      header.classList.toggle("scrolled", window.scrollY > 8);
      ticking = false;
    });
  }

  window.addEventListener("scroll", onScroll, { passive: true });
  onScroll();

  /* ---- reveal on scroll -------------------------------------------------- */

  if (!reduceMotion && "IntersectionObserver" in window) {
    var revealTargets = document.querySelectorAll(
      ".section-head, .path-card, .tour, .feature-card, .claim-list article, " +
      ".choice-list article, .roadmap-row, .quickstart-split > *, .faq details, " +
      ".footer-links > div, .stat-row-lg > div"
    );

    var observer = new IntersectionObserver(function (entries) {
      entries.forEach(function (entry) {
        if (!entry.isIntersecting) return;
        entry.target.classList.add("in");
        observer.unobserve(entry.target);
      });
    }, { threshold: 0.12, rootMargin: "0px 0px -40px 0px" });

    revealTargets.forEach(function (element) {
      element.classList.add("reveal");
      observer.observe(element);
    });
  }
})();
