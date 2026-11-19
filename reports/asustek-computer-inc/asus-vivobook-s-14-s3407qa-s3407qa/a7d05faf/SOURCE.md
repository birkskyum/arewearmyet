# Hardware report asustek-computer-inc/asus-vivobook-s-14-s3407qa-s3407qa/a7d05faf

From pull request #52 (https://github.com/turbineBMW/omarchy-hardware-reports/pull/52, not merged yet) at commit bae5ccee24e7e1019dc514e869f6f2712f557be3, made with the Omarchy Arm Installer, which takes out serial numbers, the system UUID and MAC
addresses. Here the SoC serial number (SOSN) is also zeroed in the ACPI tables and each table's checksum corrected.
The device list, driver files and registry export stay in that repository.

- `acpi/`: the ACPI tables. Disassemble with `iasl -d`.
- `smbios/dmi.bin`: the SMBIOS table. Read it with `dmidecode --from-dump`.
- `edid/`: the panel's EDID.
- `firmware.json`: the firmware files of the driver packages Windows uses, by name, size and SHA-256. The firmware itself is not included.
- `hwids.json`, `report.json`, `summary.md`: the report's hardware IDs and its own record.

Copied by scripts/archive.ts.
