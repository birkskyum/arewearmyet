# Hardware report asustek-computer-inc/asus-vivobook-14-x1407qa-x1407qa/208e5f19

From https://github.com/turbineBMW/omarchy-hardware-reports/tree/8ad795bace951c22fe99ca5f28b82467eb815de2/reports/asustek-computer-inc/asus-vivobook-14-x1407qa-x1407qa/208e5f19, made with the Omarchy Arm Installer, which takes out serial numbers, the system UUID and MAC
addresses. Here the SoC serial number (SOSN) is also zeroed in the ACPI tables and each table's checksum corrected.
The device list, driver files and registry export stay in that repository.

- `acpi/`: the ACPI tables. Disassemble with `iasl -d`.
- `smbios/dmi.bin`: the SMBIOS table. Read it with `dmidecode --from-dump`.
- `edid/`: the panel's EDID.
- `firmware.json`: the firmware files of the driver packages Windows uses, by name, size and SHA-256. The firmware itself is not included.
- `hwids.json`, `report.json`, `summary.md`: the report's hardware IDs and its own record.

Copied by scripts/archive.ts.
