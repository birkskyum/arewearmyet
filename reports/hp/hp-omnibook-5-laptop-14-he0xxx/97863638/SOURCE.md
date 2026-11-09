# Hardware report hp/hp-omnibook-5-laptop-14-he0xxx/97863638

From pull request #30 (https://github.com/turbineBMW/omarchy-hardware-reports/pull/30, not merged yet) at commit a38ea3d9bc063818aded48ad43f02860365beead, made with the Omarchy Arm Installer, which takes out serial numbers, the system UUID and MAC
addresses. Here the SoC serial number (SOSN) is also zeroed in the ACPI tables and each table's checksum corrected.
It still had the processor's or the panel's serial number, which is blanked here.
The device list, driver files and registry export stay in that repository.

- `acpi/`: the ACPI tables. Disassemble with `iasl -d`.
- `smbios/dmi.bin`: the SMBIOS table. Read it with `dmidecode --from-dump`.
- `edid/`: the panel's EDID.
- `firmware.json`: the firmware files of the driver packages Windows uses, by name, size and SHA-256. The firmware itself is not included.
- `hwids.json`, `report.json`, `summary.md`: the report's hardware IDs and its own record.

Copied by scripts/archive.ts.
