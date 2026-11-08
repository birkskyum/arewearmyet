# ASUS Vivobook S14 (S3407QA)

ACPI tables the owner, Jonas Lindeburg, dumped on his S3407QA and published with his device tree on 2026-09-13. FPDT1.dat is the same table as FPDT.dat and is left out. Disassemble with `iasl -d dsdt.dat`.

The DSDT's SOSN, the chip's serial number, is zeroed here and the table's checksum fixed.

The DSDT matches the Vivobook S16 S3607QA's (reports/asustek-computer-inc/asus-vivobook-s-16-s3607qa-s3607qa) except for the SKU (S34), the GPU and audio subsystem IDs (13241043) and some thermal limits.

From the same owner's boot log:

- `dmi.txt` is the kernel's DMI line from his first boot on Ubuntu.
- `hwids.json` holds the two hardware IDs made from it, under the compatible of his tree. The board vendor in the first one is what the Vivobook S16 S3607QA reports on the same BIOS build. omacom/omarchy-pkgs#2315 carries them in Omarchy's kernel package.
