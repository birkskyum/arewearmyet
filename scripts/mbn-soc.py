#!/usr/bin/env python3
"""Report which SoC ids a Qualcomm MBN is signed for.

The hash-table segment of a signed ELF/MBN carries the signing attributes.
Header versions 6, 7 and 8 store up to twelve SoC hardware versions in the
QTI and OEM metadata (the soc_hw_vers / soc_version array). Versions 3 and 5
store the same attributes as OU fields of the attestation certificate, and a
SOC_VERS field there is reported when the certificate has one.

Nothing here checks the signature. It only reads the attributes the signer
wrote next to it, which is what the SoC's PAS compares the chip id against.
"""
import argparse, hashlib, re, struct, sys

NUM_SOC = 12
NUM_SERIAL = 8
HASH_SEG = 2
OS_TYPE_MASK = 0x07000000
ELF_MAGIC = b"\x7fELF"

# v6 metadata, 120 bytes, from sectools parsegen/mbn/versions/v6/headers.py
# major, minor, sw_id, hw_id, oem_id, model_id, app_id, flags,
# soc_vers[12], serials[8] as u32, root cert index, anti-rollback
V6_META = struct.Struct("<" + "I" * 8 + "I" * NUM_SOC + "I" * NUM_SERIAL + "II")
assert V6_META.size == 120

# v7/v8 authority metadata, 224 bytes (OEM metadata 2.0 / QTI metadata).
# major, minor, anti-rollback, root cert index, soc_hw_vers[12],
# feature or product-segment id, jtag id, serials[8] as u64,
# oem id, oem product id, soc lifecycle, oem lifecycle, root-hash algorithm,
# root-cert hash (64), flags
V7_META = struct.Struct("<" + "IIII" + "I" * NUM_SOC + "II" + "Q" * NUM_SERIAL + "IIIII" + "64s" + "I")
assert V7_META.size == 224

# v7 flags use 2-bit values: 0 unset, 1 false, 2 true
V7_BOUND = (
    (0, "SoC Hardware Version"),
    (2, "Serial Number"),
    (4, "OEM ID"),
    (6, "OEM Product ID"),
    (8, "JTAG ID"),
    (10, "OEM Lifecycle State"),
    (14, "OEM Root Certificate Hash"),
)
# v6 flags are single bits (FLAGS_1_O)
V6_BOUND = (
    (1, "SoC Hardware Version"),
    (2, "Serial Number"),
    (10, "Hardware ID"),
)
OU_NAMES = ("SW_ID", "HW_ID", "OEM_ID", "MODEL_ID", "DEBUG", "SOC_VERS", "SW_SIZE", "SHA256", "SHA384")
OU_RE = re.compile(rb"(0[0-9A-Fa-f])\s+([0-9A-Fa-f]{2,16})\s+([A-Z][A-Z0-9_]{1,24})")


def hex32(values):
    return [f"0x{v:X}" for v in values]


def socs_of(values):
    return [v for v in values if v]


def parse_elf_hash(data):
    """(offset, size) of the hash-table program header, or None."""
    if data[:4] != ELF_MAGIC or data[5] != 1:
        return None
    is64 = data[4] == 2
    if is64:
        e_phoff = struct.unpack_from("<Q", data, 32)[0]
        e_phentsize, e_phnum = struct.unpack_from("<HH", data, 54)
        fmt = "<IIQQQQQQ"
        i_off, i_fsz, i_flags = 2, 5, 1
    else:
        e_phoff = struct.unpack_from("<I", data, 28)[0]
        e_phentsize, e_phnum = struct.unpack_from("<HH", data, 42)
        fmt = "<IIIIIIII"
        i_off, i_fsz, i_flags = 1, 4, 6
    found = None
    for i in range(e_phnum):
        off = e_phoff + i * e_phentsize
        if off + e_phentsize > len(data):
            break
        ph = struct.unpack_from(fmt, data, off)
        if (ph[i_flags] & OS_TYPE_MASK) >> 24 == HASH_SEG and ph[i_fsz]:
            found = (ph[i_off], ph[i_fsz])
    return found


def v6_meta(blob):
    if len(blob) < V6_META.size:
        return None
    v = V6_META.unpack_from(blob)
    flags = v[7]
    return {
        "format": "v6",
        "sw_id": v[2],
        "hw_id": v[3],
        "oem_id": v[4],
        "model_id": v[5],
        "soc": socs_of(v[8:8 + NUM_SOC]),
        "bound": [name for bit, name in V6_BOUND if (flags >> bit) & 1],
        "debug": (flags >> 8) & 0x3,
    }


def v7_meta(blob):
    if len(blob) < V7_META.size:
        return None
    v = V7_META.unpack_from(blob)
    flags = v[-1]
    bound = []
    for shift, name in V7_BOUND:
        val = (flags >> shift) & 0x3
        if val == 2:
            bound.append(name)
    # 0xD00000000 is the production lifecycle state these images use
    return {
        "format": "v7",
        "major": v[0],
        "anti_rollback": v[2],
        "soc": socs_of(v[4:4 + NUM_SOC]),
        "jtag_id": v[4 + NUM_SOC + 1],
        "oem_id": v[4 + NUM_SOC + 2 + NUM_SERIAL],
        "oem_product_id": v[4 + NUM_SOC + 2 + NUM_SERIAL + 1],
        "oem_lifecycle": v[4 + NUM_SOC + 2 + NUM_SERIAL + 3],
        "bound": bound,
    }


def ou_fields(region):
    """Attestation-certificate OU attributes, as older images encode them."""
    found = {}
    for m in OU_RE.finditer(region):
        name = m.group(3).decode()
        if name in OU_NAMES and name not in found:
            found[name] = int(m.group(2), 16)
    return found


def authorities(seg):
    """(version, [(authority, metadata or None, cert region)])."""
    if len(seg) < 8:
        return None, []
    version = struct.unpack_from("<I", seg, 4)[0]
    if version == 6 and len(seg) >= 48:
        qss, qcs, image_size, hts, _r4, oss, _r5, ocs, qms, oms = struct.unpack_from("<IIIIIIIIII", seg, 8)
        o = 48
        qm, om = seg[o:o + qms], seg[o + qms:o + qms + oms]
        o += qms + oms + hts
        qcert = seg[o + qss:o + qss + qcs]
        o += qss + qcs
        ocert = seg[o + oss:o + oss + ocs]
        return version, [("QTI", v6_meta(qm) if qms else None, qcert), ("OEM", v6_meta(om) if oms else None, ocert)]
    if version in (7, 8):
        hdr = 40 if version == 7 else 56
        if len(seg) < hdr:
            return version, []
        if version == 7:
            cms, qms, oms, hts, qss, qcs, oss, ocs = struct.unpack_from("<IIIIIIII", seg, 8)
            sizes = (("QTI", qms, qss, qcs), ("OEM", oms, oss, ocs))
        else:
            cms, qms, oms, hts, qs1, qc1, qs2, qc2, os1, oc1, os2, oc2 = struct.unpack_from("<IIIIIIIIIIII", seg, 8)
            sizes = (("QTI", qms, qs1 + qs2, qc1 + qc2), ("OEM", oms, os1 + os2, oc1 + oc2))
        o = hdr + cms
        out = []
        blobs = {}
        for name, msz, _, _ in sizes:
            blobs[name] = seg[o:o + msz]
            o += msz
        o += hts
        for name, msz, ssz, csz in sizes:
            cert = seg[o + ssz:o + ssz + csz]
            o += ssz + csz
            out.append((name, v7_meta(blobs[name]) if msz else None, cert))
        return version, out
    if version == 5 and len(seg) >= 40:
        qss, qcs, image_size, hts, _r4, oss, _r5, ocs = struct.unpack_from("<IIIIIIII", seg, 8)
        o = 40 + hts
        qcert = seg[o + qss:o + qss + qcs]
        o += qss + qcs
        ocert = seg[o + oss:o + oss + ocs]
        return version, [("QTI", None, qcert), ("OEM", None, ocert)]
    if version == 3 and len(seg) >= 40:
        _reserved, _dest, _image, hts, _sig_ptr, sig, _cert_ptr, cert = struct.unpack_from("<IIIIIIII", seg, 8)
        o = 40 + hts + sig
        return version, [("OEM", None, seg[o:o + cert])]
    return version, []


def describe(data):
    loc = parse_elf_hash(data)
    if loc is None:
        return {"error": "no hash-table segment"}
    off, size = loc
    if off + size > len(data):
        return {"error": f"hash segment {off:#x}+{size:#x} runs past end of file"}
    version, auths = authorities(data[off:off + size])
    rows = []
    soc = []
    for name, meta, cert in auths:
        row = {"authority": name}
        if meta:
            row.update(meta)
            soc.extend(meta["soc"])
        ou = ou_fields(cert)
        if ou:
            row["ou"] = {k: f"0x{v:X}" for k, v in ou.items()}
            if "SOC_VERS" in ou:
                soc.append(ou["SOC_VERS"])
        if meta or ou:
            rows.append(row)
    # first occurrence wins, later duplicates drop
    seen = []
    for v in soc:
        if v not in seen:
            seen.append(v)
    return {"version": version, "soc": seen, "authorities": rows}


def main(argv):
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument("files", nargs="*", help="MBN or ELF images")
    p.add_argument("--json", action="store_true", help="print one JSON object per file")
    p.add_argument("--self-test", action="store_true", help="check the header layouts and exit")
    args = p.parse_args(argv)
    if args.self_test:
        self_test()
        return 0
    if not args.files:
        p.error("a file is required")
    rc = 0
    for path in args.files:
        blob = open(path, "rb").read()
        info = describe(blob)
        info["file"] = path
        info["sha256"] = hashlib.sha256(blob).hexdigest()
        if args.json:
            import json
            print(json.dumps(info, indent=2))
        else:
            if info.get("error"):
                print(f"{path}: {info['error']}")
                rc = 1
                continue
            soc = " ".join(hex32(info["soc"])) or "(none)"
            print(f"{path}: hash v{info['version']} soc {soc}")
            for row in info["authorities"]:
                extra = []
                if row.get("soc"):
                    extra.append("soc " + " ".join(hex32(row["soc"])))
                if row.get("bound"):
                    extra.append("bound " + ",".join(row["bound"]))
                if "oem_lifecycle" in row:
                    extra.append(f"oem_lifecycle 0x{row['oem_lifecycle']:X}")
                if row.get("sw_id") is not None and row.get("format") == "v6":
                    extra.append(f"sw_id 0x{row['sw_id']:X}")
                if row.get("ou"):
                    extra.append("ou " + " ".join(f"{k}={v}" for k, v in row["ou"].items()))
                print(f"  {row['authority']}: " + ("; ".join(extra) or "no signing attributes"))
    return rc


def self_test():
    """A v7 hash segment whose OEM metadata lists 0xA016 is reported as such."""
    meta = bytearray(V7_META.size)
    # oem lifecycle 2 is production in the 32-bit field these headers carry
    V7_META.pack_into(meta, 0, 2, 0, 0, 0, 0xA016, *([0] * (NUM_SOC - 1)), 0, 0, *([0] * NUM_SERIAL), 0, 0, 0, 2, 0, b"\x00" * 64, 2)
    # header: image id, version 7, common metadata size 24, qti 0, oem 224, then zeros
    header = struct.pack("<IIIIIIIIII", 0, 7, 24, 0, V7_META.size, 0, 0, 0, 0, 0)
    seg = header + b"\x00" * 24 + bytes(meta)
    # ELF32 with one hash program header pointing at the segment
    phoff = 52
    ph = struct.pack("<IIIIIIII", 0, len(seg) and 0, 0, 0, 0, 0, 0, 0)  # placeholder, rewritten below
    elf = bytearray(b"\x7fELF\x01\x01\x01" + b"\x00" * 9)
    elf += struct.pack("<HHIIIIIHHHHHH", 2, 0xA4, 1, 0, phoff, 0, 0, 52, 32, 1, 0, 0, 0)
    assert len(elf) == phoff
    filesz_off = phoff + 16  # p_filesz is the 5th u32 of an ELF32 program header
    # rebuild the program header properly: type, offset, va, pa, filesz, memsz, flags, align
    seg_off = phoff + 32
    flags = HASH_SEG << 24
    ph = struct.pack("<IIIIIIII", 0, seg_off, 0, 0, len(seg), len(seg), flags, 4)
    blob = bytes(elf) + ph + seg
    info = describe(blob)
    assert info["version"] == 7, info
    assert info["soc"] == [0xA016], info
    oem = next(row for row in info["authorities"] if row["authority"] == "OEM")
    assert oem["oem_lifecycle"] == 2 and "SoC Hardware Version" in oem["bound"], oem
    # and a v6 metadata blob lists its soc_vers array
    raw = bytearray(V6_META.size)
    V6_META.pack_into(raw, 0, 0, 0, 0x17, 0, 0, 0, 0, 1 << 1, 0xA009, *([0] * (NUM_SOC - 1)), *([0] * NUM_SERIAL), 0, 0)
    got = v6_meta(raw)
    assert got["soc"] == [0xA009] and "SoC Hardware Version" in got["bound"], got


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
