// Adds an "Edit this page" link to the end of every page's article.
var configKeys = ["repo", "branch"];

function init(config) {
  return {
    repo: config.repo ?? "https://github.com/Pinta365/tryck",
    branch: config.branch ?? "main",
  };
}

function onPageRendered(page, state) {
  const url = `${state.repo}/edit/${state.branch}/docs/content/${page.sourcePath}`;
  const link = `<p class="edit-link"><a href="${url}">Edit this page on GitHub</a></p>`;
  page.html = page.html.replace("</article>", link + "\n</article>");
}
