# trackr

```
  ___            __        __        
   |  |__|  /\  /  ` |__/ |__/   
   |  |  | /~~\ \__, |  \ |  \   
```

**trackr = caveman tool. me show you all thing on computer. me find. me size. me kill.**

---

## WHAT TRACKR DO (the grunt explanation)

You install many thing. pip thing. npm thing. big .exe thing. Then you forget.
Thing pile up. Disk get full. Cave get messy. You no remember uninstall command.

trackr look everywhere — package manager, Windows registry, every folder on every
drive. trackr show you EVERYTHING with size and exact place it live. Then trackr
help you smash it clean, with dry-run first so you no smash wrong rock.

One file. `trackr.exe`. No installer. No runtime. You drop in PATH. You run. Done.

---

## ME GET TRACKR (install)

1. Download `trackr.exe`.
2. Put `trackr.exe` in folder.
3. Add folder to PATH (so cave-shell find it anywhere):
   - Windows Settings → "Edit the system environment variables" → Environment
     Variables → edit `Path` → add your folder.
4. Open new terminal. Type `trackr`. trackr talk back. Good.

---

## ME USE TRACKR (commands)

### `trackr scan` — show me all thing

```
trackr scan
```

trackr scan pip, npm, yarn, pnpm, docker, AND every installed .exe in registry,
AND every folder in Program Files on every drive. Shows name, version, size, place.
Big spinner spin while trackr dig. Arrow key up/down to look. `q` to leave.

Color meaning:
- GREEN = clean. has uninstall path. no problem.
- YELLOW = no uninstall string. you clean by hand.
- RED = orphan. something broken (folder gone or no registry).
- GRAY = package manager thing. small. less scary.

Flags for scan:
```
trackr scan --orphans     # show ONLY the broken leftover thing
trackr scan --json        # spit raw JSON (for robot / script)
trackr scan --sort=size   # biggest first (this is default)
trackr scan --sort=name   # alphabet order
```

### `trackr where <name>` — where thing hide?

```
trackr where nodejs
```

trackr find every place `nodejs` live: registry key, install folder + size,
pip/npm location, PATH entry. trackr add up total footprint. If many match,
trackr show list, you pick with arrow key.

### `trackr remove <name>` — smash thing (careful!)

```
trackr remove nodejs
```

trackr ALWAYS show DRY RUN first. dry run = "this what me WOULD do, nothing dead yet":
- what command me run
- what folder me delete + size
- what registry key me remove
- total disk me free

Then trackr ask `Proceed? [y/N]`. Nothing happen unless you type `y`. trackr no
trick you. After uninstaller run, if folder still there, trackr ask again before delete.

### `trackr log` — what me track so far

```
trackr log
```

Show install history trackr remember (date, tool, package, why-tag).

---

## SAFETY ROCK (hard rules trackr never break)

trackr NEVER delete from these place, no matter what:
- `C:\Windows\`, `C:\Windows\System32\`, `C:\Windows\SysWOW64\`
- `C:\Program Files\Common Files\` (shared bone)
- any drive root like `C:\` or `D:\`
- any shared install root like `C:\Program Files\` itself
- registry key that not under `Uninstall\`

If thing need admin power, trackr say "run as administrator". trackr no crash when
one tool missing or one folder locked — trackr skip, keep going, tell you at bottom.

---

## ME BUILD FROM SOURCE (for tool-maker caveman)

Need Go (latest stable). Then:

```bash
# run without build
go run . scan

# make the real .exe (Windows, 64-bit, small/stripped)
GOOS=windows GOARCH=amd64 go build -ldflags="-s -w" -o trackr.exe .
```

`-s -w` flag strip debug fat. binary stay small (~8 MB).

Stack inside trackr:
- Go (single .exe, fast)
- spf13/cobra (commands)
- charmbracelet/bubbletea + lipgloss (color, spinner, list)
- modernc.org/sqlite (pure-Go database, no DLL)
- golang.org/x/sys/windows/registry (read/kill registry)

Database live at `%USERPROFILE%\.trackr\trackr.db`.

---

## THING TRACKR CANNOT DO (no get angry)

- trackr is **Windows only**. no Linux. no Mac. by design.
- trackr **cannot remember past**. install you do BEFORE trackr exist = no history.
  `scan` still see them (current state), but `log` only track from first trackr use.
- no GUI. no cloud. no auto-update. no phone-home. no spy. trackr quiet and local.
- store/UWP app — trackr point them out but say "use winget to remove".

---

## LICENSE

MIT. take it. use it. no cry to me.
