# Images in issues and comments

How to embed images in GitHub issue bodies and comments from the CLI.

## Ask before any remote write

Every method below changes remote state: a branch and one commit per file (method 1), a gist (method 2), or the body of
an issue or comment (method 3, `--attach`). This is a write under the skill's write-safety rule. Read and verify the
target first. A remote write the user named in the request runs once that verification shows the target is the one the
user named. A remote write nobody named takes one confirmation before it runs. In both cases, state the plan first:

- the repository, or the gist, that will receive the files
- the branch name and each file path (method 1), including that the branch must stay for the images to render
- the number of commits it will create
- who can see the result: anyone with access to the repository (method 1), or anyone who has the URL of a public gist
  (method 2). An attachment (method 3) is visible to anyone who has its URL when the repository is public, and only to
  people with access to the repository when it is private or internal

A yes to one method does not cover another. Reading the default branch and base64-encoding a local file change nothing
remote, so those steps need no confirmation.

`{owner}` and `{repo}` in the `gh api` commands below are filled in by `gh` from the repository of the current
directory. When the target is another repository, write them out and pass it to `gh repo view` as
`gh repo view owner/repo`.

## Images in a pull request body

`gh pr create` and `gh pr edit` take no `--attach`. A request that asks for screenshots in a pull request, or a request
to open a draft pull request for a UI change when before and after screenshots have already been captured in the
session, covers attaching them under Verification: state the upload plan above once in the same reply and run it in this
order. Create the draft pull request with an `[open: screenshots]` marker under Verification, upload the files with
method 1 or the browser upload of method 3, then edit the body to replace the marker with the actual markdown image
links (leaving zero unresolved placeholders once uploaded). When neither upload method can run, write the body with an
`[open: screenshots]` marker under Verification and tell the user to drop the files into the description in the browser.
Never report an image as attached until the body shows its URL.

## Methods (ranked by reliability)

### 1. GitHub Contents API (recommended for private repos)

Push image files to a branch in the same repo. Then reference them with a URL that works for authenticated viewers.

**Step 1: Create a branch**

```bash
# Resolve the default branch instead of assuming its name
DEFAULT_BRANCH=$(gh repo view --json defaultBranchRef --jq '.defaultBranchRef.name')

# Get the SHA of the default branch
SHA=$(gh api "repos/{owner}/{repo}/git/ref/heads/$DEFAULT_BRANCH" --jq '.object.sha')

# Create a new branch using the resolved GitHub login (never literal "<login>")
LOGIN=$(gh api user --jq '.login')
BRANCH="${LOGIN}/images"
gh api repos/{owner}/{repo}/git/refs -X POST \
  -f ref="refs/heads/$BRANCH" \
  -f sha="$SHA"
```

**Step 2: Upload images via Contents API**

```bash
# Base64-encode the image and upload
# macOS
BASE64=$(base64 -i /path/to/image.png)
# Linux
# BASE64=$(base64 -w 0 /path/to/image.png)

gh api repos/{owner}/{repo}/contents/docs/images/my-image.png \
  -X PUT \
  -f message="Add image" \
  -f content="$BASE64" \
  -f branch="$BRANCH" \
  --jq '.content.path'
```

Repeat for each image. The Contents API creates a commit per file.

**Step 3: Reference in markdown**

Resolve `{host}`, `{owner}`, `{repo}`, and `$BRANCH` (`${LOGIN}/images`) from `gh repo view` and
`gh api user --jq '.login'` before writing the PR body—never leave literal `<login>`, `<owner>`, `<repo>`, or `<branch>`
in a published URL:

```markdown
![Description](https://${HOST}/${OWNER}/${REPO}/raw/${BRANCH}/docs/images/my-image.png)
```

`{host}` is the resolved forge host.

> Use the `https://{host}/<owner>/<repo>/raw/<branch>/<path>` format, not `raw.githubusercontent.com`. The
> `raw.githubusercontent.com` URLs return 404 for private repos, even with a valid token in the URL. The
> `{host}/.../raw/...` format works because the browser sends auth cookies when the viewer is logged in and has repo
> access.

### 2. Gist hosting (public images only)

Upload images as files in a gist. Use this only for images you are willing to make public.

```bash
# Create a gist with a placeholder file
gh gist create --public -f description.md <<< "Image hosting gist"

# Note: gh gist edit does NOT support binary files.
# You must use the API to add binary content to gists.
```

> Gists do not accept binary file uploads from the CLI. The base64-text workaround does not render as an image. Avoid
> this method.

### 3. GitHub-hosted attachment: CLI `--attach` or browser upload (most reliable rendering)

GitHub stores the file and returns a `https://github.com/user-attachments/assets/{UUID}` URL. The URL keeps working
until the attachment is deleted, and then it returns 404. On a public repository it works for anyone, without signing
in, and the image renders in email notifications. On a private or internal repository only people with access to the
repository can view it, and email notifications show a link in place of the image.

**From the CLI** (`gh` 2.99.0 or later; `gh issue create --help`, `gh issue edit --help` and `gh issue comment --help`
document the flag): `--attach` uploads a local image or video and writes its URL into the body of the issue or comment
you are already writing. The upload is part of that write, so the confirmation above covers the issue or comment text
and the files together.

In each call, capture the repository you resolved and confirmed into `$target_repo`, or read it from its scratch record.
Pass it as `--repo "$target_repo"`. `gh` fills `{owner}` and `{repo}` only in `gh api` paths. Omitting `--repo` sends
the write to the current checkout's repository. Write the issue title to a scratch file with the file-write tool and
load its location into `$title_file` in the create call.

```bash
# Create an issue with an image; the text after # is the alt text
issue_title=$(cat -- "$title_file") || exit 1
gh issue create --repo "$target_repo" --title "$issue_title" --body-file body.md --attach './login.png#The login error state'

# Comment with two images
gh issue comment 123 --repo "$target_repo" --body "Before and after" --attach ./before.png --attach ./after.png

# Add an image to an existing issue; its body is kept and the image is appended
gh issue edit 123 --repo "$target_repo" --attach ./login.png
```

If `body.md` already contains `![alt](./login.png)`, `gh` rewrites that reference to the uploaded URL. Otherwise it
appends the image. One command takes up to 50 files. When some uploads fail, the issue or comment is still written with
the ones that succeeded, the command exits non-zero, and the URL is still printed. Read the result before you report
success.

**From the browser:**

1. Open the issue/comment in a browser
2. Drag-drop or paste the image into the comment editor
3. GitHub generates the `user-attachments` URL

> Do not call GitHub's `upload/policies/assets` endpoint yourself. It requires a browser session (CSRF token + cookies)
> and returns an HTML error page for API tokens. `--attach` does the upload for you.

## Take screenshots programmatically

Use `puppeteer-core` with local Chrome to screenshot HTML mockups. Install it in a new, empty directory outside the
repository, never in the checkout. In the checkout, npm reads the project's `.npmrc`, which can point the install at
another registry, and it runs the lifecycle scripts of the packages it installs. Pin the version, name the registry, and
turn the scripts off:

```bash
mkdir shot-tool && cd shot-tool
npm install --ignore-scripts --registry=https://registry.npmjs.org puppeteer-core@24.10.0
```

Save the script below in that directory as `shot.mjs` (the `.mjs` extension lets it use `import` and top-level `await`).
Run it there with `CHROME_PATH` set to the Chrome executable and the path of the mockup HTML file as the argument:

```bash
# macOS:   CHROME_PATH="/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"
# Linux:   CHROME_PATH=/usr/bin/google-chrome
# Windows: CHROME_PATH="C:\Program Files\Google\Chrome\Application\chrome.exe"
CHROME_PATH=/usr/bin/google-chrome node shot.mjs /path/to/mockup.html
```

The mockup is a file someone else may have written, so the script renders it with JavaScript off and aborts every
request except `data:` URLs. A mockup must therefore carry its styles and images inline. This keeps a script, image,
frame, or stylesheet URL in the file from reading a local or private-network address into the screenshot.

```javascript
import { readFileSync } from "node:fs";
import puppeteer from "puppeteer-core";

const htmlString = readFileSync(process.argv[2], "utf8");

const browser = await puppeteer.launch({
    executablePath: process.env.CHROME_PATH,
    defaultViewport: { width: 900, height: 600, deviceScaleFactor: 2 },
});

const page = await browser.newPage();

// Render the file as static markup: no scripts, and no request other than an inline data: URL.
await page.setJavaScriptEnabled(false);
await page.setRequestInterception(true);
page.on("request", (request) => (request.url().startsWith("data:") ? request.continue() : request.abort()));

await page.setContent(htmlString);

// Screenshot specific elements
const elements = await page.$$(".section");
for (let i = 0; i < elements.length; i++) {
    await elements[i].screenshot({ path: `mockup-${i + 1}.png` });
}

await browser.close();
```

> MCP Playwright may not connect to localhost because of network isolation. Use puppeteer-core with a local Chrome
> installation instead.

## Quick reference

| Method                                                  | Private repos |   Permanent   |  No auth needed   |    API-only    |
| ------------------------------------------------------- | :-----------: | :-----------: | :---------------: | :------------: |
| Contents API + `{host}/…/raw/`                          |      Yes      |      Yes      |        No         |      Yes       |
| `user-attachments` via browser drag-drop                |      Yes      | Until deleted | Public repos only |       No       |
| `user-attachments` via `gh ... --attach` (`gh` 2.99.0+) |      Yes      | Until deleted | Public repos only |   Yes (CLI)    |
| `raw.githubusercontent.com`                             |   No (404)    |      Yes      |        No         |      Yes       |
| Gist                                                    |  Public only  |      Yes      |        Yes        | No (no binary) |

## Common pitfalls

- **API download URLs are temporary.** URLs returned by `gh api repos/.../contents/...` with `download_url` include a
  token that expires.
- **Base64 encoding for large files** can hit API payload limits. The Contents API has a ~100MB file size limit, but
  base64-encoded payloads hit lower practical limits.
- **Email notifications** do not render images that require authentication. If email readability matters, use the
  `user-attachments` upload (method 3).
