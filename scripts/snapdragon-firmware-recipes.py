#!/usr/bin/env python3
# Write a recipe per Snapdragon machine from arewearmyet's firmware table: each recipe downloads
# the OEM's own Windows driver package at build time, unpacks it without running it, installs the
# firmware files the machine's device tree asks for and checks each against its SHA-256.
#
# Usage: scripts/snapdragon-firmware-recipes.py --pkgbuilds DIR [--table FILE] [--linux-firmware FILE] [recipe ...]
# DIR is the pkgbuilds/ folder of an omacom/omarchy-pkgs checkout. The table defaults to
# https://arewearmyet.com/firmware-sources.json, which serves data/firmware-sources.yaml.
# --linux-firmware is the package database whose linux-firmware-qcom the recipes leave files to,
# by default Omarchy's edge mirror of Arch Linux ARM's core; "none" leaves nothing out.
# Naming recipes limits the run to them.
import argparse, io, json, os, re, sys, tarfile, textwrap, urllib.request

parser = argparse.ArgumentParser()
parser.add_argument("--table", default="https://arewearmyet.com/firmware-sources.json")
parser.add_argument("--pkgbuilds", required=True)
parser.add_argument("--linux-firmware", default="https://arm-mirror.omarchy.org/aarch64/core/core.files")
parser.add_argument("recipes", nargs="*")
args = parser.parse_args()
if re.match(r"https?://", args.table):
    request = urllib.request.Request(args.table, headers={"User-Agent": "omarchy-pkgs snapdragon-firmware-recipes"})
    with urllib.request.urlopen(request) as response:
        table = json.load(response)
else:
    table = json.load(open(args.table))
out = os.path.realpath(args.pkgbuilds)


def fetch(url):
    request = urllib.request.Request(url, headers={"User-Agent": "omarchy-pkgs snapdragon-firmware-recipes"})
    with urllib.request.urlopen(request) as response:
        return response.read()


# The firmware linux-firmware-qcom ships, by name without a compression suffix, so the recipes leave
# out what it has. Its file list comes from a pacman files database.
def linux_firmware_names(source):
    if source == "none":
        return set()
    data = fetch(source) if re.match(r"https?://", source) else open(source, "rb").read()
    with tarfile.open(fileobj=io.BytesIO(data), mode="r:*") as db:
        member = next((m for m in db.getmembers() if re.fullmatch(r"linux-firmware-qcom-[^/]+/files", m.name)), None)
        if member is None:
            raise SystemExit(f"{source} has no linux-firmware-qcom")
        lines = db.extractfile(member).read().decode().splitlines()
    names = set()
    for line in lines:
        m = re.fullmatch(r"usr/lib/firmware/(qcom/.+?)(\.zst|\.xz)?", line)
        if m and not line.endswith("/"):
            names.add(m.group(1))
    return names


in_linux_firmware = linux_firmware_names(args.linux_firmware)

# A DSP's image and its device-tree image have to come from the same release
DSP_SETS = {"audio DSP": re.compile(r"(qcadsp\d+\.mbn|adsp_dtbs?\.(elf|mbn))"), "compute DSP": re.compile(r"(qccdsp\d+\.mbn|cdsp_dtbs?\.(elf|mbn))")}

TABLE_URL = "https://arewearmyet.com/firmware-sources.json"
SCRIPT_URL = "https://github.com/birkskyum/arewearmyet/blob/main/scripts/snapdragon-firmware-recipes.py"

# Recipe name -> the machines it is for, and the firmware folders their device trees load from,
# plus an ath11k or ath12k folder for a Wi-Fi card whose board data linux-firmware doesn't list.
# Left out: glymur/ASUSTeK/UX3607OA, which asus-zenbook-a16-firmware ships, and
# x1e80100/LENOVO/21N1, whose ThinkPad T14s Gen 6 set linux-firmware already ships.
RECIPES = {
    "acer-swift-14-go-pro-ai-firmware": ("Acer Swift 14 Go Pro AI", ["qcom/x1e80100/ACER/SFA14-11"]),
    "acer-aspire-14-ai-firmware": ("Acer Aspire 14 AI", ["qcom/x1p42100/ACER/A14-11M"]),
    "acer-aspire-16-ai-firmware": ("Acer Aspire 16 AI", ["qcom/x1p42100/ACER/A16-11M"]),
    "acer-swift-14-ai-firmware": ("Acer Swift 14 AI", ["qcom/x1e80100/ACER/SF14-11"]),
    "acer-swift-go-14-firmware": ("Acer Swift Go 14", ["qcom/x1e80100/ACER/SFG14-01"]),
    "asus-proart-pz13-firmware": ("ASUS ProArt PZ13", ["qcom/x1p42100/ASUSTeK/proart-pz13"]),
    "asus-proart-pz14-firmware": ("ASUS ProArt PZ14", ["qcom/glymur/ASUSTeK/HT7407NA"]),
    "asus-vivobook-s14-firmware": ("ASUS Vivobook S 14", ["qcom/x1p42100/ASUSTeK/vivobook-s14"]),
    "asus-vivobook-s15-firmware": ("ASUS Vivobook S 15", ["qcom/x1e80100/ASUSTeK/vivobook-s15", "qcom/x1p42100/ASUSTeK/vivobook-s15"]),
    "asus-vivobook-16-firmware": ("ASUS Vivobook 16", ["qcom/x126100/ASUSTeK/vivobook-16"]),
    "asus-vivobook-s16-firmware": ("ASUS Vivobook S 16", ["qcom/x1p42100/ASUSTeK/vivobook-s16"]),
    "asus-zenbook-14-ux3480qa-firmware": ("ASUS Zenbook 14 UX3480QA", ["qcom/x1p42100/ASUSTeK/zenbook-14-ux3480qa"]),
    "asus-zenbook-a14-firmware": ("ASUS Zenbook A14", ["qcom/x1e80100/ASUSTeK/zenbook-a14", "qcom/x1p42100/ASUSTeK/zenbook-a14"]),
    "asus-zenbook-a14-ux3407na-firmware": ("ASUS Zenbook A14 UX3407NA", ["qcom/glymur/ASUSTeK/UX3407NA"]),
    "dell-inspiron-14-5441-firmware": ("Dell Inspiron 14 5441", ["qcom/x1e80100/dell/inspiron-14-plus-5441", "qcom/x1p42100/dell/inspiron-14-plus-5441"]),
    "dell-inspiron-14-plus-7441-firmware": ("Dell Inspiron 14 Plus 7441", ["qcom/x1e80100/dell/inspiron-14-plus-7441"]),
    "dell-latitude-5455-firmware": ("Dell Latitude 5455", ["qcom/x1e80100/dell/latitude-5455"]),
    "dell-latitude-7455-firmware": ("Dell Latitude 7455", ["qcom/x1e80100/dell/latitude-7455"]),
    "dell-xps-13-9345-firmware": ("Dell XPS 13 9345", ["qcom/x1e80100/dell/xps13-9345"]),
    "honor-magicbook-art-14-firmware": ("Honor MagicBook Art 14", ["qcom/x1e80100/HONOR/MRO-XXX"]),
    "hp-elitebook-6-g1q-firmware": ("HP EliteBook 6 G1q", ["qcom/x1p42100/hp/elitebook-6-g1q"]),
    "hp-elitebook-ultra-g1q-firmware": ("HP EliteBook Ultra G1q", ["qcom/x1e80100/hp/elitebook-ultra-g1q"]),
    "hp-elitebook-x-g2q-firmware": ("HP EliteBook X G2q", ["qcom/glymur/hp/elitebook-x-g2q"]),
    "hp-omnibook-3-14-firmware": ("HP OmniBook 3 14", ["qcom/x1p42100/hp/omnibook-3"]),
    "hp-omnibook-3-16-firmware": ("HP OmniBook 3 16", ["qcom/x1p42100/hp/omnibook-3-16-bz0"]),
    "hp-omnibook-5-firmware": ("HP OmniBook 5 14 and 16", ["qcom/x1p42100/hp/omnibook-5"]),
    "hp-omnibook-5-16-bf1-firmware": ("HP OmniBook 5 16 (2026)", ["qcom/glymur/HP/omnibook-5-16-bf1xxx"]),
    "hp-omnibook-ultra-14-firmware": ("HP OmniBook Ultra 14", ["qcom/glymur/hp/omnibook-ultra-14-kg0xxx"]),
    "hp-omnibook-x14-firmware": ("HP OmniBook X 14", ["qcom/x1e80100/hp/omnibook-x14", "qcom/x1p42100/hp/omnibook-x14", "ath11k/WCN6855/hw2.1"]),
    "hp-omnibook-x16-firmware": ("HP OmniBook X 16 (2026)", ["qcom/glymur/hp/omnibook-x16"]),
    "huawei-matebook-e-go-firmware": ("Huawei MateBook E Go", ["qcom/sc8280xp/HUAWEI/gaokun3"]),
    "lenovo-ideacentre-mini-x-firmware": ("Lenovo IdeaCentre Mini x", ["qcom/x1e80100/LENOVO/91B6", "qcom/x1p42100/LENOVO/91B6"]),
    "lenovo-thinkcentre-neo-50q-qc-firmware": ("Lenovo ThinkCentre neo 50q QC", ["qcom/x1p42100/LENOVO/13C8"]),
    "lenovo-thinkpad-t14s-gen7-firmware": ("Lenovo ThinkPad T14s Gen 7", ["qcom/glymur/LENOVO/22A8"]),
    "medion-sprchrgd-14-s1-firmware": ("Medion SPRCHRGD 14 S1", ["qcom/x1e80100/Medion/sprchrgd-14-s1"]),
    "microsoft-surface-laptop-7-firmware": ("Microsoft Surface Laptop 7", ["qcom/x1e80100/microsoft/Romulus", "qcom/x1e80100/microsoft"]),
    "microsoft-surface-laptop-13-firmware": ("Microsoft Surface Laptop 13-inch", ["qcom/x1p42100/Microsoft/SurfaceLaptop13"]),
    "microsoft-surface-laptop-8-firmware": ("Microsoft Surface Laptop 8", ["qcom/glymur/microsoft/mahua"]),
    "microsoft-surface-pro-9-5g-firmware": ("Microsoft Surface Pro 9 with 5G", ["qcom/sc8280xp/MICROSOFT/SurfacePro9"]),
    "microsoft-windows-dev-kit-2023-firmware": ("Microsoft Windows Dev Kit 2023", ["qcom/sc8280xp/microsoft/blackrock"]),
    "microsoft-surface-pro-11-firmware": ("Microsoft Surface Pro 11th Edition", ["qcom/x1e80100/microsoft/Denali", "qcom/x1e80100/microsoft"]),
    "microsoft-surface-pro-12in-firmware": ("Microsoft Surface Pro 12-inch", ["qcom/x1p42100/Microsoft/Surface12"]),
    "qualcomm-snapdragon-dev-kit-firmware": ("Qualcomm Snapdragon Dev Kit for Windows", ["qcom/x1e80100/Thundercomm/DEVKIT"]),
    "samsung-galaxy-book4-edge-firmware": ("Samsung Galaxy Book4 Edge", ["qcom/x1e80100/SAMSUNG/galaxy-book4-edge"]),
    "samsung-galaxy-book4-edge-15-firmware": ("Samsung Galaxy Book4 Edge 15", ["qcom/x1p42100/SAMSUNG/galaxy-book4-edge"]),
}
# Both Surfaces load the same GPU zap shader from qcom/x1e80100/microsoft.
CONFLICTS = {
    "microsoft-surface-laptop-7-firmware": ["microsoft-surface-pro-11-firmware"],
    "microsoft-surface-pro-11-firmware": ["microsoft-surface-laptop-7-firmware"],
}
# Recipes that took over another package's name. Each provides and replaces the old name, so pacman
# moves installed machines to it on the next update, and conflicts with it, so both never install.
RENAMED = {"asus-zenbook-a14-ux3407na-firmware": ["asus-zenbook-a14-x2-firmware"]}
# Device-tree compatibles for the provided.d list of firmware the board's boot firmware loads.
COMPATIBLE = {"hp-elitebook-x-g2q-firmware": "hp,elitebook-x-g2q"}

folders = {f["path"]: f for f in table["folders"]}
packages = {p["id"]: p for p in table["packages"]}

# Everything below goes into shell code, so each value the recipes take from the downloaded table
# has to look like what it is before it is written anywhere.
PATTERNS = {
    "id": r"[a-z0-9][a-z0-9._-]*",
    "maker": r"[A-Za-z][A-Za-z ]*",
    "url": r"https://[A-Za-z0-9.-]+/[A-Za-z0-9._~%/()+,=&?:-]*",
    "size": r"[0-9]+",
    "sha256": r"[0-9a-f]{64}",
    "fetched": r"[0-9]{4}-[0-9]{2}-[0-9]{2}",
    "path": r"(qcom|ath11k|ath12k)(/[A-Za-z0-9._-]+)+",
    "name": r"[A-Za-z0-9._-]+",
    # Members only reach the recipe single-quoted, and Lenovo's installers name folders like
    # code$GetExtractPath$, so anything printable but quotes and backslashes
    "member": r"[^/\x00-\x1f'\"`\\]+(/[^/\x00-\x1f'\"`\\]+)*",
}


def valid(kind, value, where):
    if not isinstance(value, (str, int)) or not re.fullmatch(PATTERNS[kind], str(value)) or ".." in str(value).split("/"):
        raise SystemExit(f"{where}: unexpected {kind} {value!r} in the firmware table")
    return value


for p in packages.values():
    where = f"package {p.get('id')!r}"
    for key in ("id", "maker", "url", "size", "sha256", "fetched"):
        valid(key, p.get(key), where)
    if p.get("page") is not None:
        valid("url", p["page"], where)
for f in folders.values():
    valid("path", f.get("path"), "folder")
    for entry in f.get("files", []):
        where = f"{f['path']}/{entry.get('name')}"
        valid("name", entry.get("name"), where)
        if entry.get("from"):
            valid("sha256", entry.get("sha256"), where)
        for src in entry.get("from", []):
            valid("id", src.get("package"), where)
            valid("member", src.get("member"), where)
    for link in f.get("links", []):
        valid("name", link.get("name"), f["path"])
        valid("path", "qcom/" + link.get("target", "").replace("../", ""), f"{f['path']}/{link.get('name')}")


def resolve(path, name, depth=0):
    """The table's entry for a file, following linux-firmware style links between folders."""
    folder = folders[path]
    for f in folder.get("files", []):
        if f["name"] == name:
            return f
    for link in folder.get("links", []):
        if link["name"] == name and depth < 4:
            target = os.path.normpath(os.path.join(path, link["target"]))
            return resolve(os.path.dirname(target), os.path.basename(target), depth + 1)
    raise KeyError(f"{path}/{name}")


def unpack_kind(pkg):
    path = pkg["url"].split("?")[0].lower()
    if path.endswith(".msi"):
        return "msi"
    if path.endswith(".exe") and pkg["maker"] == "Lenovo":
        return "inno"
    if path.endswith(".exe") and pkg["maker"] == "ASUS":
        return "7z-appended"
    return "7z"


def ext(pkg):
    m = re.search(r"(\.[A-Za-z0-9]+)$", pkg["url"].split("?")[0])
    return m.group(1).lower() if m else ".bin"


def q(s):
    return "'" + s.replace("'", "'\"'\"'") + "'"


def human_list(items):
    return items[0] if len(items) == 1 else ", ".join(items[:-1]) + " and " + items[-1]


# What the files run on, named the way the comment and description say it.
def contents(dests):
    base = [os.path.basename(d) for d in dests]
    parts = []
    if any(b.startswith(("qcadsp", "adsp", "battmgr")) for b in base):
        parts.append("audio DSP")
    if any(b.startswith(("qccdsp", "cdsp")) for b in base):
        parts.append("compute DSP")
    if any(b.startswith("qcdxkmsuc") for b in base):
        parts.append("GPU")
    if any(b.startswith("qcvss") for b in base):
        parts.append("video")
    return human_list(parts)


def is_board(dest):
    return re.match(r"ath1[12]k/", dest) is not None and os.path.basename(dest) == "board.bin"


def comment(text):
    return ["# " + line for line in textwrap.wrap(text, 96, break_on_hyphens=False)]


def recipe(name, who, paths):
    wanted, provided = [], []
    for path in paths:
        folder = folders[path]
        for n in [f["name"] for f in folder.get("files", [])] + [l["name"] for l in folder.get("links", [])]:
            if re.fullmatch(r"soccp(_dtb)?\.mbn", n):
                # The board's boot firmware starts the SoC control processor before Linux.
                provided.append(f"{path}/{n}")
                continue
            entry = resolve(path, n)
            if not entry.get("from"):
                raise SystemExit(f"{name}: {path}/{n} is in no OEM download")
            wanted.append({"dest": f"{path}/{n}", "sha256": entry["sha256"], "from": entry["from"]})

    # Leave out what linux-firmware ships. A DSP's two images go together, and when linux-firmware has
    # one but not the other, the kernel would load images from two releases, so that needs a decision.
    for path in paths:
        for dsp, pattern in DSP_SETS.items():
            in_set = [w["dest"] for w in wanted if os.path.dirname(w["dest"]) == path and pattern.fullmatch(os.path.basename(w["dest"]))]
            shipped = [d for d in in_set if d in in_linux_firmware]
            if shipped and len(shipped) < len(in_set):
                raise SystemExit(f"{name}: linux-firmware ships {', '.join(shipped)} but not the rest of the {dsp} set in {path}")
    left_out = [w["dest"] for w in wanted if w["dest"] in in_linux_firmware]
    wanted = [w for w in wanted if w["dest"] not in in_linux_firmware]
    if not wanted:
        print(f"{name}: linux-firmware ships every file, so the recipe can go")
        return None

    # The fewest OEM downloads that hold every file: the one with the most of the rest first,
    # the smaller on a tie.
    remaining, chosen = list(range(len(wanted))), []
    while remaining:
        counts = {}
        for i in remaining:
            for s in wanted[i]["from"]:
                counts[s["package"]] = counts.get(s["package"], 0) + 1
        best = min(counts, key=lambda p: (-counts[p], packages[p]["size"], p))
        chosen.append(best)
        for i in list(remaining):
            src = next((s for s in wanted[i]["from"] if s["package"] == best), None)
            if src:
                wanted[i]["src"] = (best, src["member"])
                remaining.remove(i)

    for w in wanted:
        if "!xz@" in w["src"][1]:
            raise SystemExit(f"{name}: {w['dest']} is an XZ stream; the generator does not unpack those")

    # The audio DSP is the image signed for the board; the others can be Qualcomm's generic ones from
    # another maker's download, so the signer and the recipe's page come from the audio DSP's package.
    signed = [w["src"][0] for w in wanted if re.match(r"(qcadsp\d+\.mbn|adsp_dtbs\.elf)$", os.path.basename(w["dest"]))]
    signers = [p for p in chosen if p in signed] or chosen
    makers = sorted({packages[p]["maker"] for p in signers})
    kinds = {p: unpack_kind(packages[p]) for p in chosen}
    makedepends = ["7zip"] + (["innoextract"] if "inno" in kinds.values() else []) + (["msitools"] if "msi" in kinds.values() else [])
    local = {p: f"{p}{ext(packages[p])}" for p in chosen}
    version = max(packages[p]["fetched"] for p in chosen).replace("-", "")
    page = next((packages[p].get("page") for p in signers if packages[p].get("page")), None) or packages[signers[0]]["url"].split("?")[0]

    what = contents([w["dest"] for w in wanted if not is_board(w["dest"])])
    boards = [w["dest"] for w in wanted if is_board(w["dest"])]
    title = what[0].upper() + what[1:] + " firmware" + (" and Wi-Fi board data" if boards else "")
    L = []
    L += comment(
        f"{title} for the {who}. The firmware only runs if it is signed for this "
        f"board by {human_list(makers)}, so linux-firmware cannot carry it until the OEM submits it. The OEM ships it "
        "in its Windows driver download, which this recipe unpacks without running. Each file is checked against the "
        "SHA-256 it was matched to and installed under the name the device tree requests, compressed with xz. The "
        "kernel tries a plain name, then .zst, then .xz, so once linux-firmware ships the same file, its copy is the "
        "one loaded, and the two packages never install the same path.")
    if boards:
        L += ["#"] + comment(
            "The Wi-Fi board data is the file the OEM's Windows Wi-Fi driver loads for this laptop's card, "
            "which linux-firmware's board-2.bin doesn't list. The driver looks the card up in board-2.bin first "
            "and loads board.bin only when no record matches, so a record added to linux-firmware takes over, "
            "and ath12k-board-fallback leaves the card to this file.")
    if provided:
        L += ["#"] + comment(
            "The board's own firmware starts the SoC control processor before Linux, so its images are not "
            "installed, and provided.d tells qcom-firmware-extract not to look for them.")
    L += ["#", f"# Generated from {TABLE_URL} by", f"# {SCRIPT_URL};", "# change the table, not this file.", ""]
    L += [
        f"pkgname={name}",
        f"pkgver={version}",
        "pkgrel=@PKGREL@",
        f"pkgdesc={q(title + ' for the ' + who)}",
        "arch=('aarch64')",
        f"url={q(page)}",
        "license=('LicenseRef-proprietary')",
        "provides=(" + " ".join([q("qcom-firmware-" + p.removeprefix("qcom/").replace("/", "-").lower()) for p in paths if p.startswith("qcom/")]
                                + [q(old) for old in RENAMED.get(name, [])]) + ")",
    ]
    conflicts = CONFLICTS.get(name, []) + RENAMED.get(name, [])
    if conflicts:
        L.append("conflicts=(" + " ".join(q(c) for c in conflicts) + ")")
    if name in RENAMED:
        L.append("replaces=(" + " ".join(q(old) for old in RENAMED[name]) + ")")
    L += [
        "makedepends=(" + " ".join(q(d) for d in makedepends) + ")",
        "options=('!strip' '!debug')",
    ]
    if any(packages[p]["maker"] == "Dell" for p in chosen):
        L += [
            "# dl.dell.com turns away clients that do not name a browser.",
            "DLAGENTS=('https::/usr/bin/curl -qgb \"\" -A Mozilla/5.0 -fLC - --retry 3 --retry-delay 3 -o %o %u')",
        ]
    L.append("source=(")
    for p in chosen:
        L.append(f'  {q(local[p] + "::" + packages[p]["url"])}')
    L.append(")")
    if provided:
        L.append(f"source+=({q(COMPATIBLE[name] + '.list')})")
    L.append("noextract=(" + " ".join(q(local[p]) for p in chosen) + ")")
    L.append("sha256sums=(" + " ".join(q(packages[p]["sha256"]) for p in chosen) + ")")
    if provided:
        L.append("sha256sums+=('SKIP')")
    L += [
        "",
        "# Unpack an OEM download the way it is packed: MSIs with msiextract, Lenovo's installers with",
        "# innoextract, ASUS's installers from the 7-Zip archive appended to them, the rest with 7z.",
        "_unpack() {",
        "  local file=$1 dir=$2 kind=$3 offset",
        '  rm -rf "$dir"',
        '  mkdir -p "$dir"',
        "  case $kind in",
        '    msi) msiextract -C "$dir" "$file" >/dev/null ;;',
        '    inno) innoextract -s -d "$dir" "$file" ;;',
        "    7z-appended)",
        "      offset=$(LC_ALL=C grep -obUaP '7z\\xBC\\xAF\\x27\\x1C' \"$file\" | head -1 | cut -d: -f1)",
        '      [[ -n $offset ]] || { echo "no 7z archive in $file" >&2; return 1; }',
        '      tail -c +$((offset + 1)) "$file" >"$dir.7z"',
        '      7z x -y -o"$dir" "$dir.7z" >/dev/null',
        "      ;;",
        '    *) 7z x -y -o"$dir" "$file" >/dev/null ;;',
        "  esac",
        "}",
        "",
        "prepare() {",
    ]
    for p in chosen:
        L.append(f"  _unpack {q(local[p])} {q(p)} {kinds[p]}")
    nested = sorted({(w["src"][0], "!/".join(w["src"][1].split("!/")[:i])) for w in wanted for i in range(1, len(w["src"][1].split("!/")))}, key=lambda t: (t[0], t[1].count("!/"), t[1]))
    for p, inner in nested:
        disk = inner.replace("!/", "__x/")
        L.append(f"  _unpack {q(p + '/' + disk)} {q(p + '/' + disk + '__x')} 7z")
    L += ["}", "", "package() {", '  local fw="$pkgdir/usr/lib/firmware"']
    for w in sorted(wanted, key=lambda w: w["dest"]):
        p, member = w["src"]
        L.append(f"  install -Dm644 {q(p + '/' + member.replace('!/', '__x/'))} \"$fw\"/{q(w['dest'])}")
    L += ["", "  # The bytes each file was matched to in the OEM's download.", "  (", '    cd "$fw"', "    sha256sum -c --quiet <<'SUMS'"]
    for w in sorted(wanted, key=lambda w: w["dest"]):
        L.append(f"{w['sha256']}  {w['dest']}")
    L += ["SUMS", "  )", "", '  find "$fw" -type f -exec xz -C crc32 {} +']
    if provided:
        L += ["", f'  install -Dm644 -t "$pkgdir/usr/share/qcom-firmware-extract/provided.d" {q(COMPATIBLE[name] + ".list")}']
    L += ["}", ""]

    d = os.path.join(out, name)
    os.makedirs(os.path.join(d, ".omarchy"), exist_ok=True)
    # A changed recipe of the same version gets the next pkgrel, so it is published again
    text, pkgrel = "\n".join(L), 1
    old_path = os.path.join(d, "PKGBUILD")
    if os.path.exists(old_path):
        old = open(old_path).read()
        old_ver = re.search(r"^pkgver=(.*)$", old, re.M)
        old_rel = re.search(r"^pkgrel=(\d+)$", old, re.M)
        if old_ver and old_rel and old_ver.group(1) == version:
            pkgrel = int(old_rel.group(1))
            if re.sub(r"^pkgrel=\d+$", "pkgrel=@PKGREL@", old, flags=re.M) != text:
                pkgrel += 1
    open(old_path, "w").write(text.replace("pkgrel=@PKGREL@", f"pkgrel={pkgrel}"))
    open(os.path.join(d, ".omarchy", "package.json"), "w").write('{\n  "source": "local"\n}\n')
    if provided:
        open(os.path.join(d, COMPATIBLE[name] + ".list"), "w").write(
            "# The board's own firmware starts the SoC control processor before Linux.\n" + "\n".join(provided) + "\n")
    return {"name": name, "files": len(wanted), "left_out": left_out, "sources": [(p, packages[p]["size"]) for p in chosen], "provided": provided}


for name, (who, paths) in RECIPES.items():
    if args.recipes and name not in args.recipes:
        continue
    r = recipe(name, who, paths)
    if r is None:
        continue
    print(f'{r["name"]:40} {r["files"]:3} files from ' + ", ".join(f"{p} ({s // 1000000} MB)" for p, s in r["sources"]) + (f'  provided: {len(r["provided"])}' if r["provided"] else "") + (f'  left to linux-firmware: {len(r["left_out"])}' if r["left_out"] else ""))
