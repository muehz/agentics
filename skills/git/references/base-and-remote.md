# Base and Remote Resolver

## Overview

This file holds the one procedure that discovers the base branch, the base remote, the push remote and the protected
branches of a repository. It has a Bash form and a PowerShell 7 form, and both print the same result.

Every git mode that needs one of these names follows this file. Any other skill that needs a base or a remote links
here, runs the procedure, and takes the lines it needs.

[repo-conventions.md](../../routing/references/repo-conventions.md#pointer-section) gives the repository a
`Canonical remote:` pointer and a `Base branch:` pointer, and links this procedure as its discovery step. The block
itself reads no pointer. The caller reads them, as [How to call it](#how-to-call-it) says.

## How to call it

1. Copy the block for your shell: [Bash](#bash-procedure) or [PowerShell](#powershell-procedure).
2. Fill in line 1 with the base the user named, or the base the caller recorded for the work item. Write the branch name
   as the base remote spells it, with no remote prefix. Leave it empty when there is none.
3. Leave line 2 as `user` when the user named the base. Set it to `recorded` when the caller passes a recorded base.
4. Fill in line 3 with a base remote the user named, and line 4 with a push remote the user named. Each takes a remote
   name. Leave each empty when the user named none.
5. Read the `Canonical remote:` and `Base branch:` pointers from a trusted revision, never from the branch being
   resolved, which another person may have written
   ([untrusted-data.md](../../routing/references/contracts/untrusted-data.md)). Run the block once as steps 6 and 7 say,
   then read the instruction files as they stand at its `base-ref`, not in the working tree: the base a user named or
   recorded, the base the host reports for the branch's open pull request, or the default branch of the base remote.
   With `base-ref` unresolved or absent locally, read no pointer. When those files hold a `Canonical remote:` pointer
   and the user named no base remote, find the remote whose URL points at the pointer's repository, put that remote's
   name on line 3 and run the block again. Report the output's `source: user` as coming from the pointer. A pointer wins
   over discovery, as [repo-conventions.md](../../routing/references/repo-conventions.md#precedence) says, but it never
   replaces a host-reported pull-request base without the user's confirmation: when the first run reads
   `base-remote: … source: pull-request` and the pointer names another repository, ask the user once which one is the
   base, naming both. When no remote points at it, do not run discovery in its place: ask the user once which remote is
   the canonical one, naming the pointer's URL.
6. Run the block inline in your shell tool, as written, from inside the repository. Do not save it to a script file
   first. It prints the result lines on standard output and nothing else.
7. Read the lines by name. Ask only the `question` lines for the names you use, in one round, and leave the others
   unasked, except as step 8 says.
8. When the instruction files at the step 5 revision hold a `Base branch:` pointer and the output reads
   `base-branch: … source: remote-default` or `base-branch: unresolved`, run the block again with the pointer on line 1
   and `recorded` on line 2, and do not ask the base branch question. Report the rerun's `source: recorded` as coming
   from the pointer. The pointer stands in only for the remote's default branch, as
   [repo-conventions.md](../../routing/references/repo-conventions.md#base-branch) says, so a base the user named, a
   recorded base or a pull-request base is never replaced.
9. After the user answers, fill in the matching input line and run the block again. A base branch answer goes on line 1,
   a base remote answer on line 3, and a push remote answer on line 4. The answered line then reads `source: user`.

An answer the user gives holds for the session. Keep the answers, fill them in on every later run, and do not ask the
same question again.

A base branch question the user has not answered stays open even when the base remote is answered. Do not take the
`remote-default` branch the next run fills in as the answer: ask only the open base branch question and hold the
operation until the user answers it. Recommend, for the base remote, the remote the current pull request targets or else
the remote that holds the default branch; for the base branch, the pull request's base from
`gh pr view --json baseRefName` or else the remote default.

Every line is resolved for the current branch, so run the block again after any switch to another branch.

When `fetched` reads that the ref is absent locally, the printed `base-ref` does not exist. Stop and report it.

The block turns off git's and `gh`'s own terminal prompts while it runs, so a missing credential fails the read and does
not wait for input. It does not suppress an SSH passphrase or host-key prompt, or an askpass helper.

The block leaves the shell as it found it, apart from the input variables on lines 1 to 4 and, in PowerShell, the exit
code variable, which it sets to 0. The Bash form runs in a subshell. The PowerShell form runs in a child scope and
restores the three environment variables it sets. Neither form depends on the caller's error options.

The block ends successfully whenever it printed the result, including a result that reads `unresolved`, and it works on
a detached `HEAD`.

The block runs one `git fetch`, of the base branch only. The fetch names the base's remote-tracking ref as its
destination, so it writes that ref even when the remote's fetch mapping leaves the base out, as in a single-branch
clone. It updates that ref and `FETCH_HEAD`, and the block changes nothing else in the repository. Read `FETCH_HEAD`
only from a later fetch of your own. A caller that needs its own branch from the push remote fetches that branch itself.

## Result

The result is six lines, always in this order. Each line states one name and where it came from.

```text
base-branch: <name> | unresolved                    source: user | recorded | pull-request | remote-default | none
base-remote: <remote> | <url> | none | unresolved   source: pull-request | host-parent | only-remote | user | none
base-ref:    <remote>/<name> | <sha> (fetched from <url>) | <name> (local) | unresolved   fetched: yes | no (<reason>)
push-remote: <remote> | none | unresolved           source: push-config | upstream | only-remote | user | none
protected:   <branch>[, <branch>...] | none         host: read | not consulted (<reason>)
question:    <one question> | none
```

- **`unresolved`** means the repository is silent on that name. The caller asks the user.
- **`none`** on a remote line means the repository has no remote.
- **`source`** reads `none` when its line reads `none` or `unresolved`.
- **`question`** repeats, once for each line that reads `unresolved`, in the order base remote, base branch, push
  remote. A caller asks them in one round. With nothing to ask, the result holds one line, `question: none`.

With no remote, the base branch is the one the user named or the caller recorded, and `base-ref` is that local branch
with nothing fetched. When neither exists, `base-branch` and `base-ref` are `unresolved` and the question asks which
local branch is the base.

When the base remote is a URL, a fetch writes no remote-tracking ref. `base-ref` is then the commit the fetch of that
branch returned, printed as its SHA. Compare against that SHA.

## Resolution order

Each name is the first source in its list that exists.

- **Base branch.**
    1. A base the user named, or one the caller passes as recorded for the work item.
    2. The base of the branch's open pull request.
    3. The default branch of the base remote, read from that remote's `HEAD`.

    Otherwise it is `unresolved`. Compare against the base as `base-ref`, after the fetch of the base remote. When the
    fetch fails, the `fetched` field says so and names the stale ref that was used.

- **Base remote.**
    1. The remote whose URL is the open pull request's base repository.
    2. The remote whose URL is the parent repository the host reports for a fork.
    3. The only remote.

    When the host reports a fork parent that no remote points to, the result is the parent's URL, and the default branch
    and the fetch use that URL. With one remote and a host that cannot be read, the result is that remote and the
    `protected` line says the host was not consulted. With several remotes and none of these readable, it is
    `unresolved`.

- **Push remote.**
    1. The branch's configured push remote. This is the remote git's own push configuration selects for that branch.
    2. The remote of the branch's upstream.
    3. The only remote.

    Otherwise it is `unresolved`.

- **Protected set.** The default branch of every remote, the resolved base branch, and any branch the host reports as
  protected. Each branch is listed once. When the host cannot be read, the first two still apply.

- **Host reads.** The pull request base, the fork parent and branch protection come from `gh`. The procedure calls `gh`
  only when it is installed, it is signed in, and a remote URL names a host it is signed in to. A local path names no
  host, so the procedure decides that from the URL before any `gh` call. Otherwise it skips those sources, and the
  `host` field of the `protected` line gives the reason and names each source it skipped.

- **Default branch.** The procedure asks the remote itself for its `HEAD`, so a clone with no local record of the remote
  still resolves. When the remote cannot be reached, it uses the local record of that `HEAD` when one exists.

- **No literal names.** No command in the procedure, or anywhere else in this skill, names `origin`, `main` or `master`.
  Remote names and branch names are read from the repository.

- **Shells.** Every path is quoted, and remote, branch and worktree lists are read as whole lines.

## Bash procedure

```bash
named_base=''
named_source='user'
named_base_remote=''
named_push_remote=''
(
  # The caller's error options must not end the block early, and nothing set here outlives the subshell.
  set +e +u
  set +o pipefail
  export GIT_TERMINAL_PROMPT=0 GCM_INTERACTIVE=never GH_PROMPT_DISABLED=1

  lower() { printf '%s' "$1" | tr '[:upper:]' '[:lower:]'; }

  # Prints the default branch of a remote name or URL, or nothing when it cannot be read.
  default_of() {
    local line
    line=$(git ls-remote --symref "$1" HEAD 2>/dev/null | head -n 1) || true
    case "$line" in
      "ref: refs/heads/"*)
        line=${line#ref: refs/heads/}
        printf '%s\n' "${line%%[[:space:]]*}"
        return 0
        ;;
    esac
    line=$(git symbolic-ref --quiet "refs/remotes/$1/HEAD" 2>/dev/null) || return 0
    printf '%s\n' "${line#"refs/remotes/$1/"}"
  }

  # Prints the remote whose host repository is "$1", or nothing.
  remote_of() {
    local index=0 want name
    want=$(lower "$1")
    for name in "${remotes[@]}"; do
      if [ -n "${fulls[$index]}" ] && [ "$(lower "${fulls[$index]}")" = "$want" ]; then
        printf '%s\n' "$name"
        return 0
      fi
      index=$((index + 1))
    done
  }

  add_protected() {
    local have
    [ -n "$1" ] || return 0
    for have in "${protected[@]}"; do
      [ "$have" = "$1" ] && return 0
    done
    protected+=("$1")
  }

  remotes=()
  while IFS= read -r line; do
    [ -n "$line" ] && remotes+=("$line")
  done <<<"$(git remote)"
  count=${#remotes[@]}
  branch=$(git branch --show-current 2>/dev/null)

  url_form='^(https?|ssh|git)://([^/@]*@)?([^/:]+)(:[0-9]+)?/(.+)$'
  scp_form='^([^/@:]+@)?([^/@:][^/@:]+):(.+)$'
  urls=() hosts=() repos=() defaults=() fulls=() parents=() parent_urls=()
  remote_list=
  for remote in "${remotes[@]}"; do
    url=$(git remote get-url "$remote" 2>/dev/null)
    host_name='' repo=''
    if [[ $url =~ $url_form ]]; then
      host_name=${BASH_REMATCH[3]} repo=${BASH_REMATCH[5]}
    elif [[ $url != *://* && $url =~ $scp_form ]]; then
      host_name=${BASH_REMATCH[2]} repo=${BASH_REMATCH[3]}
    fi
    repo=${repo#/}
    repo=${repo%/}
    repo=${repo%.git}
    host_name=$(lower "$host_name")
    default_branch=$(default_of "$remote")
    urls+=("$url") hosts+=("$host_name") repos+=("$repo") defaults+=("$default_branch")
    fulls+=('') parents+=('') parent_urls+=('')
    remote_list=${remote_list:+$remote_list, }$remote
  done

  push_remote='' push_source=none push_ask=''
  if [ -z "$named_push_remote" ]; then
    [ -n "$branch" ] && push_remote=$(git config --get "branch.${branch}.pushRemote" || true)
    [ -n "$push_remote" ] || push_remote=$(git config --get remote.pushDefault || true)
  fi
  if [ -n "$named_push_remote" ]; then
    push_remote=$named_push_remote push_source=user
  elif [ -n "$push_remote" ]; then
    push_source=push-config
  else
    [ -n "$branch" ] && push_remote=$(git config --get "branch.${branch}.remote" || true)
    [ "$push_remote" = . ] && push_remote=''
    if [ -n "$push_remote" ]; then
      push_source=upstream
    elif [ "$count" -eq 1 ]; then
      push_remote=${remotes[0]} push_source=only-remote
    elif [ "$count" -eq 0 ]; then
      push_remote=none
    else
      push_remote=unresolved push_ask=1
    fi
  fi

  gh_host='' host_note='' missed='' any_host='' tried=' '
  for host_name in "${hosts[@]}"; do
    [ -n "$host_name" ] && any_host=1
  done
  if [ "$count" -eq 0 ]; then
    host_note='no remote'
  elif [ -z "$any_host" ]; then
    host_note='no remote is on a GitHub host'
  elif ! command -v gh >/dev/null 2>&1; then
    host_note='gh is not installed'
  else
    for host_name in "${hosts[@]}"; do
      [ -n "$host_name" ] || continue
      case "$tried" in *" $host_name "*) continue ;; esac
      tried="$tried$host_name "
      if gh auth status --hostname "$host_name" >/dev/null 2>&1; then
        gh_host=$host_name
        break
      fi
    done
    [ -n "$gh_host" ] || host_note='gh is not signed in to the host of any remote'
  fi

  pr_repo='' pr_base='' host_protected='' parent_failed=''
  if [ -n "$gh_host" ]; then
    index=0
    for remote in "${remotes[@]}"; do
      if [ "${hosts[$index]}" = "$gh_host" ] && [ -n "${repos[$index]}" ]; then
        fulls[$index]=${repos[$index]}
        if out=$(gh api --hostname "$gh_host" "repos/${repos[$index]}" --jq '[.full_name, .parent.full_name, .parent.clone_url, .parent.ssh_url] | @tsv' 2>/dev/null); then
          IFS=$'\t' read -r full parent parent_https parent_ssh <<<"${out%$'\r'}"
          [ -n "$full" ] && fulls[$index]=$full
          parents[$index]=$parent
          case "${urls[$index]}" in
            http://* | https://*) parent_urls[$index]=$parent_https ;;
            *) parent_urls[$index]=$parent_ssh ;;
          esac
        else
          parent_failed=1
        fi
      fi
      index=$((index + 1))
    done
    [ -n "$parent_failed" ] && missed='fork parent'

    if [ -n "$branch" ]; then
      seen='|' pr_failed=''
      for base in "${parents[@]}" "${fulls[@]}"; do
        [ -n "$base" ] || continue
        for full in "${fulls[@]}"; do
          [ -n "$full" ] || continue
          head_ref="${full%%/*}:${branch}"
          case "$seen" in *"|$base $head_ref|"*) continue ;; esac
          seen="$seen$base $head_ref|"
          if out=$(gh api --hostname "$gh_host" --method GET "repos/$base/pulls" -f state=open -f "head=$head_ref" --jq '.[0] | select(. != null) | [.base.repo.full_name, .base.ref] | @tsv' 2>/dev/null); then
            if [ -n "$out" ]; then
              IFS=$'\t' read -r pr_repo pr_base <<<"${out%$'\r'}"
              break 2
            fi
          else
            pr_failed=1
          fi
        done
      done
      if [ -z "$pr_repo" ] && [ -n "$pr_failed" ]; then
        missed=${missed:+$missed, }'pull request base'
      fi
    fi

    seen='|' protection_failed=''
    for full in "${fulls[@]}" "${parents[@]}"; do
      [ -n "$full" ] || continue
      case "$seen" in *"|$full|"*) continue ;; esac
      seen="$seen$full|"
      if out=$(gh api --hostname "$gh_host" --paginate --method GET "repos/$full/branches" -f protected=true -f per_page=100 --jq '.[].name' 2>/dev/null); then
        host_protected="$host_protected$out"$'\n'
      else
        protection_failed=1
      fi
    done
    [ -n "$protection_failed" ] && missed=${missed:+$missed, }'branch protection'
    [ -n "$missed" ] && host_note='gh could not read every source'
  else
    missed='pull request base, fork parent, branch protection'
  fi

  # base_kind is name, url, none or unresolved.
  base_remote=$named_base_remote base_source=none base_kind=''
  [ -n "$base_remote" ] && base_source=user base_kind=name
  if [ -z "$base_remote" ] && [ -n "$pr_repo" ]; then
    base_remote=$(remote_of "$pr_repo")
    [ -n "$base_remote" ] && base_source=pull-request base_kind=name
  fi
  if [ -z "$base_remote" ]; then
    index=0
    for remote in "${remotes[@]}"; do
      if [ -n "${parents[$index]}" ]; then
        base_remote=$(remote_of "${parents[$index]}") base_source=host-parent base_kind=name
        if [ -z "$base_remote" ]; then
          base_remote=${parent_urls[$index]} base_kind=url
        fi
        break
      fi
      index=$((index + 1))
    done
  fi
  if [ -z "$base_remote" ]; then
    base_source=none
    if [ "$count" -eq 1 ]; then
      base_remote=${remotes[0]} base_source=only-remote base_kind=name
    elif [ "$count" -eq 0 ]; then
      base_remote=none base_kind=none
    else
      base_remote=unresolved base_kind=unresolved
    fi
  fi

  base_branch=$named_base branch_source=$named_source
  if [ -z "$base_branch" ]; then
    branch_source=none
    if [ -n "$pr_base" ]; then
      base_branch=$pr_base branch_source=pull-request
    elif [ "$base_kind" = url ]; then
      base_branch=$(default_of "$base_remote")
      [ -n "$base_branch" ] && branch_source=remote-default
    elif [ "$base_kind" = name ]; then
      index=0
      for remote in "${remotes[@]}"; do
        [ "$remote" = "$base_remote" ] && base_branch=${defaults[$index]}
        index=$((index + 1))
      done
      [ -n "$base_branch" ] && branch_source=remote-default
    fi
  fi

  base_ref=unresolved
  if [ "$base_kind" = none ]; then
    fetched='no (no remote)'
    [ -n "$base_branch" ] && base_ref="$base_branch (local)"
  elif [ "$base_kind" = unresolved ]; then
    fetched='no (base remote unresolved)'
  elif [ -z "$base_branch" ]; then
    fetched='no (base branch unresolved)'
  elif [ "$base_kind" = url ]; then
    if git fetch --quiet "$base_remote" "refs/heads/$base_branch" >/dev/null 2>&1 &&
      sha=$(git rev-parse --verify --quiet FETCH_HEAD); then
      base_ref="$sha (fetched from $base_remote)" fetched=yes
    else
      fetched="no (fetch from $base_remote failed, a URL has no stale ref)"
    fi
  else
    base_ref="$base_remote/$base_branch"
    if git fetch --quiet "$base_remote" "+refs/heads/$base_branch:refs/remotes/$base_remote/$base_branch" >/dev/null 2>&1; then
      fetched=yes
    elif git show-ref --verify --quiet "refs/remotes/$base_remote/$base_branch"; then
      fetched="no (fetch from $base_remote failed, stale $base_ref used)"
    else
      fetched="no (fetch from $base_remote failed, $base_ref is absent locally)"
    fi
  fi

  protected=()
  for name in "${defaults[@]}"; do
    add_protected "$name"
  done
  add_protected "$base_branch"
  while IFS= read -r line; do
    add_protected "${line%$'\r'}"
  done <<<"$host_protected"
  protected_list=''
  for name in "${protected[@]}"; do
    protected_list=${protected_list:+$protected_list, }$name
  done
  if [ -n "$gh_host" ] && [ -z "$missed" ]; then
    host_state=read
  else
    host_state="not consulted ($host_note; skipped: $missed)"
  fi

  printf 'base-branch: %s   source: %s\n' "${base_branch:-unresolved}" "$branch_source"
  printf 'base-remote: %s   source: %s\n' "$base_remote" "$base_source"
  printf 'base-ref:    %s   fetched: %s\n' "$base_ref" "$fetched"
  printf 'push-remote: %s   source: %s\n' "$push_remote" "$push_source"
  printf 'protected:   %s   host: %s\n' "${protected_list:-none}" "$host_state"
  asked=''
  if [ "$base_kind" = unresolved ]; then
    printf 'question:    Which remote holds the base branch? The remotes are: %s.\n' "$remote_list"
    asked=1
  fi
  if [ -z "$base_branch" ]; then
    if [ "$base_kind" = none ]; then
      printf 'question:    Which local branch is the base?\n'
    else
      printf 'question:    Which branch is the base?\n'
    fi
    asked=1
  fi
  if [ -n "$push_ask" ]; then
    printf 'question:    Which remote does this branch push to? The remotes are: %s.\n' "$remote_list"
    asked=1
  fi
  [ -n "$asked" ] || printf 'question:    none\n'
)
```

## PowerShell procedure

```powershell
$namedBase = ''
$namedSource = 'user'
$namedBaseRemote = ''
$namedPushRemote = ''
& {
  # A failing git or gh read is an answer here, so the caller's error options must not end the block early.
  $ErrorActionPreference = 'Continue'
  $PSNativeCommandUseErrorActionPreference = $false
  $gitPromptBefore = $env:GIT_TERMINAL_PROMPT
  $gcmPromptBefore = $env:GCM_INTERACTIVE
  $ghPromptBefore = $env:GH_PROMPT_DISABLED
  $env:GIT_TERMINAL_PROMPT = '0'
  $env:GCM_INTERACTIVE = 'never'
  $env:GH_PROMPT_DISABLED = '1'
  try {
    # Returns the default branch of a remote name or URL, or an empty string when it cannot be read.
    function Get-RemoteDefault([string]$Target) {
      $line = @(git ls-remote --symref "$Target" HEAD 2>$null) | Select-Object -First 1
      $lead = 'ref: refs/heads/'
      if ($line -and $line.StartsWith($lead, [StringComparison]::Ordinal)) {
        return ($line.Substring($lead.Length) -split '\s')[0]
      }
      $recorded = "$(git symbolic-ref --quiet "refs/remotes/$Target/HEAD" 2>$null)"
      $lead = "refs/remotes/$Target/"
      if ($recorded.StartsWith($lead, [StringComparison]::Ordinal)) { return $recorded.Substring($lead.Length) }
      return ''
    }

    $remotes = @(git remote | Where-Object { $_ })
    $count = $remotes.Count
    $branch = "$(git branch --show-current 2>$null)"

    $urls = @(); $remoteHosts = @(); $repos = @(); $defaults = @()
    $fulls = @(); $parents = @(); $parentUrls = @()
    foreach ($remote in $remotes) {
      $url = "$(git remote get-url "$remote" 2>$null)"
      $hostName = ''; $repo = ''
      if ($url -cmatch '^(?:https?|ssh|git)://(?:[^/@]*@)?([^/:]+)(?::[0-9]+)?/(.+)$') {
        $hostName = $Matches[1]; $repo = $Matches[2]
      } elseif ($url -notlike '*://*' -and $url -cmatch '^(?:[^/@:]+@)?([^/@:][^/@:]+):(.+)$') {
        $hostName = $Matches[1]; $repo = $Matches[2]
      }
      $repo = $repo -replace '^/', '' -replace '/$', '' -replace '\.git$', ''
      $urls += $url; $remoteHosts += $hostName.ToLowerInvariant(); $repos += $repo
      $defaults += "$(Get-RemoteDefault "$remote")"
      $fulls += ''; $parents += ''; $parentUrls += ''
    }
    $remoteList = $remotes -join ', '

    # Returns the remote whose host repository is $Repository, or an empty string.
    function Get-RemoteOf([string]$Repository) {
      for ($i = 0; $i -lt $count; $i++) {
        if ($fulls[$i] -and $fulls[$i] -ieq $Repository) { return $remotes[$i] }
      }
      return ''
    }

    $pushRemote = ''; $pushSource = 'none'; $pushAsk = $false
    if (-not $namedPushRemote) {
      if ($branch) { $pushRemote = "$(git config --get "branch.${branch}.pushRemote")" }
      if (-not $pushRemote) { $pushRemote = "$(git config --get remote.pushDefault)" }
    }
    if ($namedPushRemote) {
      $pushRemote = $namedPushRemote; $pushSource = 'user'
    } elseif ($pushRemote) {
      $pushSource = 'push-config'
    } else {
      if ($branch) { $pushRemote = "$(git config --get "branch.${branch}.remote")" }
      if ($pushRemote -eq '.') { $pushRemote = '' }
      if ($pushRemote) {
        $pushSource = 'upstream'
      } elseif ($count -eq 1) {
        $pushRemote = $remotes[0]; $pushSource = 'only-remote'
      } elseif ($count -eq 0) {
        $pushRemote = 'none'
      } else {
        $pushRemote = 'unresolved'; $pushAsk = $true
      }
    }

    $ghHost = ''; $hostNote = ''; $missed = @()
    $namedHosts = @($remoteHosts | Where-Object { $_ } | Select-Object -Unique)
    if ($count -eq 0) {
      $hostNote = 'no remote'
    } elseif ($namedHosts.Count -eq 0) {
      $hostNote = 'no remote is on a GitHub host'
    } elseif (-not (Get-Command gh -ErrorAction SilentlyContinue)) {
      $hostNote = 'gh is not installed'
    } else {
      foreach ($hostName in $namedHosts) {
        gh auth status --hostname "$hostName" *> $null
        if ($LASTEXITCODE -eq 0) { $ghHost = $hostName; break }
      }
      if (-not $ghHost) { $hostNote = 'gh is not signed in to the host of any remote' }
    }

    $prRepo = ''; $prBase = ''; $hostProtected = @()
    if ($ghHost) {
      for ($i = 0; $i -lt $count; $i++) {
        if ($remoteHosts[$i] -ne $ghHost -or -not $repos[$i]) { continue }
        $fulls[$i] = $repos[$i]
        $out = "$(gh api --hostname "$ghHost" "repos/$($repos[$i])" --jq '[.full_name, .parent.full_name, .parent.clone_url, .parent.ssh_url] | @tsv' 2>$null)"
        if ($LASTEXITCODE -eq 0) {
          $fields = @($out.Trim("`r", "`n") -split "`t") + @('', '', '', '')
          if ($fields[0]) { $fulls[$i] = $fields[0] }
          $parents[$i] = $fields[1]
          $parentUrls[$i] = if ($urls[$i] -cmatch '^https?://') { $fields[2] } else { $fields[3] }
        } elseif ($missed -notcontains 'fork parent') {
          $missed += 'fork parent'
        }
      }

      if ($branch) {
        $seen = @(); $prFailed = $false
        :search foreach ($base in @($parents) + @($fulls)) {
          if (-not $base) { continue }
          foreach ($full in $fulls) {
            if (-not $full) { continue }
            $headRef = "$(($full -split '/')[0]):${branch}"
            if ($seen -ccontains "$base $headRef") { continue }
            $seen += "$base $headRef"
            $out = "$(gh api --hostname "$ghHost" --method GET "repos/$base/pulls" -f state=open -f "head=$headRef" --jq '.[0] | select(. != null) | [.base.repo.full_name, .base.ref] | @tsv' 2>$null)"
            if ($LASTEXITCODE -ne 0) { $prFailed = $true; continue }
            if ($out) {
              $fields = @($out.Trim("`r", "`n") -split "`t") + @('', '')
              $prRepo = $fields[0]; $prBase = $fields[1]
              break search
            }
          }
        }
        if (-not $prRepo -and $prFailed) { $missed += 'pull request base' }
      }

      $seen = @(); $protectionFailed = $false
      foreach ($full in @($fulls) + @($parents)) {
        if (-not $full -or $seen -ccontains $full) { continue }
        $seen += $full
        $out = @(gh api --hostname "$ghHost" --paginate --method GET "repos/$full/branches" -f protected=true -f per_page=100 --jq '.[].name' 2>$null)
        if ($LASTEXITCODE -eq 0) { $hostProtected += $out } else { $protectionFailed = $true }
      }
      if ($protectionFailed) { $missed += 'branch protection' }
      if ($missed.Count -gt 0) { $hostNote = 'gh could not read every source' }
    } else {
      $missed = @('pull request base', 'fork parent', 'branch protection')
    }

    # $baseKind is name, url, none or unresolved.
    $baseRemote = $namedBaseRemote; $baseSource = 'none'; $baseKind = ''
    if ($baseRemote) { $baseSource = 'user'; $baseKind = 'name' }
    if (-not $baseRemote -and $prRepo) {
      $baseRemote = "$(Get-RemoteOf "$prRepo")"
      if ($baseRemote) { $baseSource = 'pull-request'; $baseKind = 'name' }
    }
    if (-not $baseRemote) {
      for ($i = 0; $i -lt $count; $i++) {
        if (-not $parents[$i]) { continue }
        $baseRemote = "$(Get-RemoteOf "$($parents[$i])")"; $baseSource = 'host-parent'; $baseKind = 'name'
        if (-not $baseRemote) { $baseRemote = $parentUrls[$i]; $baseKind = 'url' }
        break
      }
    }
    if (-not $baseRemote) {
      $baseSource = 'none'
      if ($count -eq 1) {
        $baseRemote = $remotes[0]; $baseSource = 'only-remote'; $baseKind = 'name'
      } elseif ($count -eq 0) {
        $baseRemote = 'none'; $baseKind = 'none'
      } else {
        $baseRemote = 'unresolved'; $baseKind = 'unresolved'
      }
    }

    $baseBranch = $namedBase; $branchSource = $namedSource
    if (-not $baseBranch) {
      $branchSource = 'none'
      if ($prBase) {
        $baseBranch = $prBase; $branchSource = 'pull-request'
      } elseif ($baseKind -eq 'url') {
        $baseBranch = "$(Get-RemoteDefault "$baseRemote")"
        if ($baseBranch) { $branchSource = 'remote-default' }
      } elseif ($baseKind -eq 'name') {
        for ($i = 0; $i -lt $count; $i++) {
          if ($remotes[$i] -ceq $baseRemote) { $baseBranch = $defaults[$i] }
        }
        if ($baseBranch) { $branchSource = 'remote-default' }
      }
    }

    $baseRef = 'unresolved'
    if ($baseKind -eq 'none') {
      $fetched = 'no (no remote)'
      if ($baseBranch) { $baseRef = "$baseBranch (local)" }
    } elseif ($baseKind -eq 'unresolved') {
      $fetched = 'no (base remote unresolved)'
    } elseif (-not $baseBranch) {
      $fetched = 'no (base branch unresolved)'
    } elseif ($baseKind -eq 'url') {
      git fetch --quiet "$baseRemote" "refs/heads/$baseBranch" *> $null
      $sha = ''
      if ($LASTEXITCODE -eq 0) {
        $sha = "$(git rev-parse --verify --quiet FETCH_HEAD)"
      }
      if ($sha) {
        $baseRef = "$sha (fetched from $baseRemote)"; $fetched = 'yes'
      } else {
        $fetched = "no (fetch from $baseRemote failed, a URL has no stale ref)"
      }
    } else {
      $baseRef = "$baseRemote/$baseBranch"
      git fetch --quiet "$baseRemote" "+refs/heads/${baseBranch}:refs/remotes/$baseRemote/$baseBranch" *> $null
      if ($LASTEXITCODE -eq 0) {
        $fetched = 'yes'
      } else {
        git show-ref --verify --quiet "refs/remotes/$baseRemote/$baseBranch"
        if ($LASTEXITCODE -eq 0) {
          $fetched = "no (fetch from $baseRemote failed, stale $baseRef used)"
        } else {
          $fetched = "no (fetch from $baseRemote failed, $baseRef is absent locally)"
        }
      }
    }

    $protected = @()
    foreach ($name in @($defaults) + @($baseBranch) + @($hostProtected)) {
      $name = "$name".Trim()
      if ($name -and $protected -cnotcontains $name) { $protected += $name }
    }
    $protectedList = if ($protected.Count -gt 0) { $protected -join ', ' } else { 'none' }
    $missedList = $missed -join ', '
    $hostState = "not consulted ($hostNote; skipped: $missedList)"
    if ($ghHost -and $missed.Count -eq 0) { $hostState = 'read' }

    $shownBranch = if ($baseBranch) { $baseBranch } else { 'unresolved' }
    $result = @(
      "base-branch: $shownBranch   source: $branchSource"
      "base-remote: $baseRemote   source: $baseSource"
      "base-ref:    $baseRef   fetched: $fetched"
      "push-remote: $pushRemote   source: $pushSource"
      "protected:   $protectedList   host: $hostState"
    )
    $questions = @()
    if ($baseKind -eq 'unresolved') {
      $questions += "question:    Which remote holds the base branch? The remotes are: $remoteList."
    }
    if (-not $baseBranch) {
      if ($baseKind -eq 'none') {
        $questions += 'question:    Which local branch is the base?'
      } else {
        $questions += 'question:    Which branch is the base?'
      }
    }
    if ($pushAsk) {
      $questions += "question:    Which remote does this branch push to? The remotes are: $remoteList."
    }
    if ($questions.Count -eq 0) { $questions += 'question:    none' }
    $result + $questions
  } finally {
    # Assigning $null removes a variable that was not set before.
    $env:GIT_TERMINAL_PROMPT = $gitPromptBefore
    $env:GCM_INTERACTIVE = $gcmPromptBefore
    $env:GH_PROMPT_DISABLED = $ghPromptBefore
  }
}
$global:LASTEXITCODE = 0
```
