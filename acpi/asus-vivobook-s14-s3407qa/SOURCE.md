# ASUS Vivobook S14 (S3407QA)

ACPI tables an owner dumped and published with their device tree in https://github.com/jonaslindeburg-svg/vivobook-s3407qa-linux, commit a9b964ee371675a8c1e5e23ea7c8da00f2178dfd (acpi/). FPDT1.dat is the same table as FPDT.dat and is left out. Disassemble with `iasl -d dsdt.dat`.

The DSDT's SOSN, the chip's serial number, is zeroed here and the table's checksum fixed.

The DSDT matches the Vivobook S16 S3607QA's (reports/asustek-computer-inc/asus-vivobook-s-16-s3607qa-s3607qa) except for the SKU (S34), the GPU and audio subsystem IDs (13241043) and some thermal limits.
