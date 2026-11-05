# Hardware report asustek-computer-inc/asus-vivobook-s-16-s3607qa-s3607qa/661c946f

From https://github.com/turbineBMW/omarchy-hardware-reports/tree/ded40129b0ec1093f3abd8980f49f431add15d8f/reports/asustek-computer-inc/asus-vivobook-s-16-s3607qa-s3607qa/661c946f, made with the Omarchy Arm Installer, which takes out serial numbers, the system UUID and MAC
addresses. Here the SoC serial number (SOSN) is also zeroed in the ACPI tables and each table's checksum corrected (1 table still had it).
The device list, driver files and registry export stay in that repository.

- `acpi/`: the ACPI tables. Disassemble with `iasl -d`.
- `smbios/dmi.bin`: the SMBIOS table. Read it with `dmidecode --from-dump`.
- `edid/`: the panel's EDID.
- `firmware.json`: the firmware files of the driver packages Windows uses, by name, size and SHA-256. The firmware itself is not included.
- `hwids.json`, `report.json`, `summary.md`: the report's hardware IDs and its own record.

Copied by scripts/archive.ts.
