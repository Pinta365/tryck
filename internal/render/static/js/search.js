// Search for the built-in theme. The index (/search-index.json) is
// fetched the first time the search box opens, and every search runs in
// the browser. A page matches when every word of the query appears in its
// title, headings or text; accents and case are ignored.
(() => {
  const root = document.querySelector(".search");
  if (!root) return;
  const dialog = root.querySelector(".search-dialog");
  const input = root.querySelector(".search-input");
  const list = root.querySelector(".search-results");
  const status = root.querySelector(".search-status");
  root.hidden = false;

  const MAX_RESULTS = 12;
  let docs = null;
  let loading = null;
  let active = -1;

  // folded returns s lowercased and without accents, plus where each
  // folded character came from in s, so matches can be marked in s.
  function folded(s) {
    let f = "";
    const from = [];
    let i = 0;
    for (const ch of s) {
      const g = ch.normalize("NFD").replace(/\p{M}/gu, "").toLowerCase();
      for (let k = 0; k < g.length; k++) from.push(i);
      f += g;
      i += ch.length;
    }
    from.push(s.length);
    return { s, f, from };
  }

  const fold = (s) => folded(s).f;

  function load() {
    loading ??= fetch(root.dataset.index)
      .then((r) => {
        if (!r.ok) throw new Error(r.statusText);
        return r.json();
      })
      .then((data) => {
        docs = data.map((d) => ({
          url: d.url,
          title: folded(d.title),
          headings: (d.headings ?? []).map((h) => ({ id: h.id, text: folded(h.text) })),
          text: folded(d.text ?? ""),
        }));
      })
      .catch(() => {
        status.textContent = "The search index could not be loaded.";
      });
    return loading;
  }

  const boundary = /[\s\p{P}]/u;

  // atWordStart reports whether term occurs in f at the start of a word.
  function atWordStart(f, term) {
    for (let i = f.indexOf(term); i >= 0; i = f.indexOf(term, i + 1)) {
      if (i === 0 || boundary.test(f[i - 1])) return true;
    }
    return false;
  }

  function count(f, term, max) {
    let n = 0;
    for (let i = f.indexOf(term); i >= 0 && n < max; i = f.indexOf(term, i + term.length)) n++;
    return n;
  }

  function search(query) {
    const terms = fold(query).split(/\s+/).filter(Boolean);
    if (!terms.length) return [];
    const hits = [];
    for (const d of docs) {
      let score = 0;
      for (const t of terms) {
        let s = 0;
        if (d.title.f.includes(t)) s += atWordStart(d.title.f, t) ? 12 : 8;
        if (d.headings.some((h) => h.text.f.includes(t))) s += 5;
        s += count(d.text.f, t, 5);
        if (s === 0) {
          score = 0;
          break;
        }
        score += s;
      }
      if (score === 0) continue;
      // Link to the first heading that contains every word, if any.
      const heading = d.headings.find((h) => terms.every((t) => h.text.f.includes(t))) ?? null;
      hits.push({ d, score, heading });
    }
    hits.sort((a, b) => b.score - a.score || a.d.title.s.localeCompare(b.d.title.s));
    return hits.slice(0, MAX_RESULTS).map((h) => ({ ...h, terms }));
  }

  const escape = (s) =>
    s.replace(/[&<>"]/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;" })[c]);

  // mark returns the part of x between folded offsets lo and hi as HTML,
  // with every occurrence of the terms wrapped in <mark>.
  function mark(x, terms, lo = 0, hi = x.f.length) {
    const ranges = [];
    for (const t of terms) {
      for (let i = x.f.indexOf(t, lo); i >= 0 && i + t.length <= hi; i = x.f.indexOf(t, i + 1)) {
        ranges.push([i, i + t.length]);
      }
    }
    ranges.sort((a, b) => a[0] - b[0]);
    let html = "";
    let at = lo;
    for (const [a, b] of ranges) {
      if (b <= at) continue;
      const start = Math.max(a, at);
      html += escape(x.s.slice(x.from[at], x.from[start]));
      html += "<mark>" + escape(x.s.slice(x.from[start], x.from[b])) + "</mark>";
      at = b;
    }
    return html + escape(x.s.slice(x.from[at], x.from[hi]));
  }

  // snippet returns about 160 characters of text around the first match.
  function snippet(x, terms) {
    const hits = terms.map((t) => x.f.indexOf(t)).filter((i) => i >= 0);
    if (!hits.length) return "";
    const first = Math.min(...hits);
    let lo = Math.max(0, first - 50);
    let hi = Math.min(x.f.length, first + 110);
    if (lo > 0) lo = x.f.indexOf(" ", lo) + 1 || lo;
    if (hi < x.f.length) hi = x.f.lastIndexOf(" ", hi) > first ? x.f.lastIndexOf(" ", hi) : hi;
    return (lo > 0 ? "… " : "") + mark(x, terms, lo, hi) + (hi < x.f.length ? " …" : "");
  }

  function render(hits, query) {
    active = -1;
    input.removeAttribute("aria-activedescendant");
    list.innerHTML = hits
      .map(({ d, heading, terms }, i) => {
        const href = heading?.id ? `${d.url}#${heading.id}` : d.url;
        return `<li id="search-result-${i}" role="option" aria-selected="false"><a href="${escape(href)}">` +
          `<span class="search-title">${mark(d.title, terms)}</span>` +
          (heading ? `<span class="search-heading">${mark(heading.text, terms)}</span>` : "") +
          `<span class="search-snippet">${snippet(d.text, terms)}</span></a></li>`;
      })
      .join("");
    input.setAttribute("aria-expanded", String(hits.length > 0));
    status.textContent = query.trim() && !hits.length ? `No results for “${query.trim()}”` : "";
    if (hits.length) select(0);
  }

  function select(i) {
    const items = list.querySelectorAll("[role=option]");
    if (!items.length) return;
    if (active >= 0) items[active].setAttribute("aria-selected", "false");
    active = (i + items.length) % items.length;
    items[active].setAttribute("aria-selected", "true");
    items[active].scrollIntoView({ block: "nearest" });
    input.setAttribute("aria-activedescendant", items[active].id);
  }

  function update() {
    if (!docs) {
      status.textContent = loading ? "Loading…" : "";
      return;
    }
    render(search(input.value), input.value);
  }

  function open() {
    if (dialog.open) return;
    dialog.showModal();
    input.select();
    if (!docs) {
      status.textContent = "Loading…";
      load().then(update);
    }
  }

  root.querySelector(".search-open").addEventListener("click", open);
  root.querySelector(".search-close").addEventListener("click", () => dialog.close());
  // A click on the backdrop lands on the dialog element itself.
  dialog.addEventListener("click", (e) => {
    if (e.target === dialog) dialog.close();
  });
  list.addEventListener("click", (e) => {
    if (e.target.closest("a")) dialog.close();
  });
  input.addEventListener("input", update);
  input.addEventListener("keydown", (e) => {
    if (e.key === "ArrowDown" || e.key === "ArrowUp") {
      e.preventDefault();
      select(active + (e.key === "ArrowDown" ? 1 : -1));
    } else if (e.key === "Enter") {
      e.preventDefault();
      const link = list.querySelectorAll("a")[Math.max(active, 0)];
      if (link) {
        dialog.close();
        location.href = link.href;
      }
    }
  });
  document.addEventListener("keydown", (e) => {
    const typing = e.target.closest?.("input, textarea, select, [contenteditable]");
    if ((e.key === "/" && !typing) || ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === "k")) {
      e.preventDefault();
      open();
    }
  });
})();
