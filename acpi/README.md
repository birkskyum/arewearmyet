# ACPI dumps

Every ACPI dump the board knows of, by model, with a link to this repository's copy. scripts/archive.ts copies the
tables of the dumps in [aarch64-laptops](https://github.com/aarch64-laptops/build), merged or proposed in a pull
request, into `aarch64-laptops/`, those of the Arm Surfaces in [linux-surface](https://github.com/linux-surface/acpidumps)
into `linux-surface/`, and the hardware reports into [`../reports/`](../reports), every day. `firmware/` holds tables
taken out of the BIOS or UEFI images in makers' own downloads, for models nobody has dumped, so they have no per-unit
values and can differ from a running machine where the firmware fills fields in at boot. The other folders here are
dumps published here first. Each folder's SOURCE.md says where its tables came from. The SoC serial number (SOSN)
is zeroed, and MSDM and SLIC, which hold the Windows product key, are left out. Disassemble a table with `iasl -d`.

90 of 115 models have at least one dump.

| Model | Dumps |
| --- | --- |
| Acer Aspire 14 AI | [hardware-reports/acer/aspire-a14-11m](../reports/acer/aspire-a14-11m) |
| Acer Aspire 16 AI | [hardware-reports/acer/aspire-a16-11m](../reports/acer/aspire-a16-11m) |
| Acer Swift 14 AI | [hardware-reports/acer/swift-sf14-11t](../reports/acer/swift-sf14-11t) |
| Acer Swift 14 Go Pro AI | [acer-sfa14-11](aarch64-laptops/acer-sfa14-11), [acer-swift-sfa14-11](aarch64-laptops/acer-swift-sfa14-11) |
| Anduril BlueChip | [hardware-reports/anduril/qualcomm-glymur](../reports/anduril/qualcomm-glymur) |
| ASUS V400 AiO (VM441QA) | [firmware/asus-v400-aio-vm441qa](firmware/asus-v400-aio-vm441qa) |
| ASUS V400 AiO (VM471QA) | [firmware/asus-v400-aio-vm471qa](firmware/asus-v400-aio-vm471qa) |
| ASUS Vivobook 14 | [firmware/asus-vivobook-14-x1407qa](firmware/asus-vivobook-14-x1407qa) |
| ASUS Vivobook S 15 | [asus-vivobook-s-15](aarch64-laptops/asus-vivobook-s-15), [asus-vivobook-s15-q5507](aarch64-laptops/asus-vivobook-s15-q5507), [asus-vivobook-s15-s5507](aarch64-laptops/asus-vivobook-s15-s5507) |
| ASUS Vivobook S14 (S3407QA) | [arewearmyet/asus-vivobook-s14-s3407qa](asus-vivobook-s14-s3407qa) |
| ASUS Vivobook S14 (S5408QA) | [firmware/asus-vivobook-s14-s5408qa](firmware/asus-vivobook-s14-s5408qa) |
| ASUS Vivobook S14 Flip | [firmware/asus-vivobook-s14-flip-tp5408qa](firmware/asus-vivobook-s14-flip-tp5408qa) |
| ASUS Vivobook S16 (S3607QA) | [hardware-reports/asustek-computer-inc/asus-vivobook-s-16-s3607qa-s3607qa](../reports/asustek-computer-inc/asus-vivobook-s-16-s3607qa-s3607qa) |
| ASUS Vivobook S16 (S5608QA) | [firmware/asus-vivobook-s16-s5608qa](firmware/asus-vivobook-s16-s5608qa) |
| ASUS Vivobook S16 Flip | [firmware/asus-vivobook-s16-flip-tp5608qa](firmware/asus-vivobook-s16-flip-tp5608qa) |
| ASUS Zenbook 14 (2026) | [asus-zenbook-14-ux3480qa](aarch64-laptops/asus-zenbook-14-ux3480qa) |
| ASUS Zenbook A14 | [asus-zenbook-a14-ux3407q](aarch64-laptops/asus-zenbook-a14-ux3407q) |
| ASUS Ascent QN10 | [asus-qn10-16e](aarch64-laptops/asus-qn10-16e) |
| ASUS ProArt PZ14 | [hardware-reports/asus/proart-pz14-ht7407na](../reports/asus/proart-pz14-ht7407na), [firmware/asus-proart-pz14-ht7407](firmware/asus-proart-pz14-ht7407) |
| ASUS Vivobook S16 (S3607NA) | [firmware/asus-vivobook-s16-s3607na](firmware/asus-vivobook-s16-s3607na) |
| ASUS Zenbook A14 (X2) | [asus-zenbook-a14-ux3407na](aarch64-laptops/asus-zenbook-a14-ux3407na) |
| ASUS Zenbook A16 | [asus-zenbook-a16-ux3607oa](aarch64-laptops/asus-zenbook-a16-ux3607oa) |
| Dell Inspiron 14 5441 | [dell-inspiron-14-5441-x1p-42-100](aarch64-laptops/dell-inspiron-14-5441-x1p-42-100), [dell-inspron-14-5441-x1p-64-100](aarch64-laptops/dell-inspron-14-5441-x1p-64-100), [hardware-reports/dell-inc/inspiron-14-5441](../reports/dell-inc/inspiron-14-5441) |
| Dell Inspiron 14 Plus 7441 | [dell-inspiron-14-7441-x1e-80-100](aarch64-laptops/dell-inspiron-14-7441-x1e-80-100) |
| Dell Latitude 5455 | [dell-latitude-5455](aarch64-laptops/dell-latitude-5455) |
| Dell Latitude 7455 | [dell-latitude-7455](aarch64-laptops/dell-latitude-7455) |
| Dell XPS 13 9345 | [dell-xps-9345](aarch64-laptops/dell-xps-9345) |
| Dynabook XD5 | [firmware/dynabook-xd5](firmware/dynabook-xd5) |
| Honor MagicBook Art 14 | [honor-mro-521](aarch64-laptops/honor-mro-521), [honor-magicbook-art-14](aarch64-laptops/honor-magicbook-art-14) |
| HP EliteBook 6 G1q | [hp-elitebook-6-g1q](aarch64-laptops/hp-elitebook-6-g1q) |
| HP EliteBook Ultra G1q | [hp-elitebook-ultra-g1q](aarch64-laptops/hp-elitebook-ultra-g1q) |
| HP OmniBook 3 16 (16-bz0) | [hardware-reports/hp/hp-omnibook-3-laptop-16-bz0xxx](../reports/hp/hp-omnibook-3-laptop-16-bz0xxx) |
| HP OmniBook 5 14 | [hp-omnibook-5-14-he0xxx](aarch64-laptops/hp-omnibook-5-14-he0xxx), [hardware-reports/hp/hp-omnibook-5-laptop-14-he0xxx](../reports/hp/hp-omnibook-5-laptop-14-he0xxx) |
| HP OmniBook X 14 | [hp-omnibook-x14-fe0xxx](aarch64-laptops/hp-omnibook-x14-fe0xxx) |
| HP EliteBook 6 G2q | [firmware/hp-elitebook-6-g2q](firmware/hp-elitebook-6-g2q) |
| HP OmniBook Ultra 14 | [arewearmyet/hp-omnibook-ultra-14-kg0](hp-omnibook-ultra-14-kg0) |
| Lenovo IdeaCentre Mini x | [lenovo-ideacentre-01q8x10](aarch64-laptops/lenovo-ideacentre-01q8x10) |
| Lenovo IdeaPad 5x 2-in-1 | [lenovo-ideapad-5-2in1-83GH-14Q8X9](aarch64-laptops/lenovo-ideapad-5-2in1-83GH-14Q8X9) |
| Lenovo IdeaPad Slim 3x | [lenovo-ideapad-slim3x-15q8x10-WCN7850](aarch64-laptops/lenovo-ideapad-slim3x-15q8x10-WCN7850) |
| Lenovo IdeaPad Slim 5x | [lenovo-ideapad-5-slim-14Q8X9](aarch64-laptops/lenovo-ideapad-5-slim-14Q8X9), [lenovo-xiaoxin-14-q8x9](aarch64-laptops/lenovo-xiaoxin-14-q8x9) |
| Lenovo IdeaPad Vibe | [firmware/lenovo-ideapad-vibe-14q8x12](firmware/lenovo-ideapad-vibe-14q8x12) |
| Lenovo IdeaPad Vibe 15 | [firmware/lenovo-ideapad-vibe-14q8x12](firmware/lenovo-ideapad-vibe-14q8x12) |
| Lenovo ThinkBook 16 G7 QOY | [lenovo-thinkbook-16-g7-qoy](aarch64-laptops/lenovo-thinkbook-16-g7-qoy) |
| Lenovo ThinkCentre neo 50q QC | [firmware/lenovo-thinkcentre-neo-50q-qc](firmware/lenovo-thinkcentre-neo-50q-qc), [hardware-reports/lenovo/13c8cto1ww](../reports/lenovo/13c8cto1ww) |
| Lenovo ThinkPad T14s Gen 6 | [lenovo-thinkpad-t14s](aarch64-laptops/lenovo-thinkpad-t14s), [lenovo-thinkpad-t14s-120hz-64gb](aarch64-laptops/lenovo-thinkpad-t14s-120hz-64gb) |
| Lenovo Yoga Slim 7x | [lenovo-yoga-slim-7x](aarch64-laptops/lenovo-yoga-slim-7x) |
| Lenovo IdeaPad 5x 2-in-1 Gen 11 | [firmware/lenovo-ideapad-5x-2-in-1-14q8y11](firmware/lenovo-ideapad-5x-2-in-1-14q8y11) |
| Lenovo IdeaPad Slim 5x 13 Gen 11 | [firmware/lenovo-ideapad-slim-5x-13q8y11](firmware/lenovo-ideapad-slim-5x-13q8y11) |
| Lenovo IdeaPad Slim 5x 15 Gen 11 | [firmware/lenovo-ideapad-slim-5x-15q8y11](firmware/lenovo-ideapad-slim-5x-15q8y11) |
| Lenovo ThinkBook 14 Gen 9 | [firmware/lenovo-thinkbook-14-gen-9-q8y](firmware/lenovo-thinkbook-14-gen-9-q8y) |
| Lenovo Yoga Slim 7x Gen 11 | [lenovo-yoga-slim-7x-14Q8Y11](aarch64-laptops/lenovo-yoga-slim-7x-14Q8Y11) |
| Medion SPRCHRGD 14 S1 | [medion-14-s1-elite-sprchrgd](aarch64-laptops/medion-14-s1-elite-sprchrgd), [medion-14-s1-sprchrgd](aarch64-laptops/medion-14-s1-sprchrgd) |
| Microsoft Surface Laptop 7 (13.8") | [microsoft-surface-romulus13_and_15](aarch64-laptops/microsoft-surface-romulus13_and_15), [linux-surface/surface_laptop_7_qcom](linux-surface/surface_laptop_7_qcom), [hardware-reports/microsoft-corporation/microsoft-surface-laptop-7th-edition](../reports/microsoft-corporation/microsoft-surface-laptop-7th-edition) |
| Microsoft Surface Laptop 7 (15") | [microsoft-surface-romulus13_and_15](aarch64-laptops/microsoft-surface-romulus13_and_15), [linux-surface/surface_laptop_7_qcom](linux-surface/surface_laptop_7_qcom) |
| Microsoft Surface Pro 11th Edition | [microsoft-surface-pro-11](aarch64-laptops/microsoft-surface-pro-11), [linux-surface/surface_pro_11_qcom](linux-surface/surface_pro_11_qcom), [hardware-reports/microsoft-corporation/microsoft-surface-pro-11th-edition](../reports/microsoft-corporation/microsoft-surface-pro-11th-edition) |
| Microsoft Surface Pro 12-inch | [microsoft-surface-pro-12-in](aarch64-laptops/microsoft-surface-pro-12-in) |
| Microsoft Surface Laptop 13-inch (2nd Edition) | [firmware/microsoft-surface-laptop-13in-2nd-edition](firmware/microsoft-surface-laptop-13in-2nd-edition) |
| Microsoft Surface Laptop 8 (13.8") | [microsoft-surface-8-x2e-80-100](aarch64-laptops/microsoft-surface-8-x2e-80-100) |
| Microsoft Surface Laptop 8 (15") | [hardware-reports/unknown/unknown/94bdf4be](../reports/unknown/unknown/94bdf4be) |
| Microsoft Surface Pro 12-inch (2nd Edition) | [firmware/microsoft-surface-pro-12in-2nd-edition](firmware/microsoft-surface-pro-12in-2nd-edition) |
| Microsoft Surface Pro 12th Edition | [firmware/microsoft-surface-pro-12th-edition](firmware/microsoft-surface-pro-12th-edition) |
| Qualcomm Snapdragon Dev Kit for Windows | [qualcomm-snapdragon-dev-kit](aarch64-laptops/qualcomm-snapdragon-dev-kit) |
| Samsung Galaxy Book4 Edge 16 | [hardware-reports/samsung-electronics-co-ltd/galaxy-book4-edge/a220b319](../reports/samsung-electronics-co-ltd/galaxy-book4-edge/a220b319) |
| Samsung Galaxy Book4 Edge 14 | [samsung-galaxybook4-edge](aarch64-laptops/samsung-galaxybook4-edge) |
| Huawei MateBook E Go | [huawei-matebooke-go](aarch64-laptops/huawei-matebooke-go) |
| Lenovo ThinkPad X13s | [lenovo-thinkpad-x13s](aarch64-laptops/lenovo-thinkpad-x13s) |
| Microsoft Surface Pro 9 5G | [microsoft-surface-pro-9-5G](aarch64-laptops/microsoft-surface-pro-9-5G), [linux-surface/surface_pro_9_5g](linux-surface/surface_pro_9_5g) |
| Microsoft Windows Dev Kit 2023 | [microsoft-devkit-2023](aarch64-laptops/microsoft-devkit-2023), [linux-surface/windows_dev_kit_2023](linux-surface/windows_dev_kit_2023) |
| Acer Aspire 1 | [acer-aspire-1](aarch64-laptops/acer-aspire-1) |
| Acer Spin 7 | [acer-spin-7-sp714-61na](aarch64-laptops/acer-spin-7-sp714-61na) |
| Apcsilmic Dot 1 | [apcsilmic-dot-1](aarch64-laptops/apcsilmic-dot-1) |
| ASUS NovaGo | [asus-tp370ql](aarch64-laptops/asus-tp370ql) |
| Dell Inspiron 14 3420 | [dell-inspiron-14-3420](aarch64-laptops/dell-inspiron-14-3420) |
| ECS LIVA QC710 | [ecs-liva-qc710](aarch64-laptops/ecs-liva-qc710) |
| Gateway GWTN133-1 | [acer-gateway-gwtn133-1](aarch64-laptops/acer-gateway-gwtn133-1) |
| HP Envy x2 (Snapdragon) | [hp-envy-x2](aarch64-laptops/hp-envy-x2) |
| Huawei MateBook E (2019) | [huawei-matebooke-2019](aarch64-laptops/huawei-matebooke-2019) |
| Lenovo 10w | [lenovo-l10w](aarch64-laptops/lenovo-l10w) |
| Lenovo Flex 5G | [lenovo-flex-5g](aarch64-laptops/lenovo-flex-5g) |
| Lenovo IdeaPad 5G | [lenovo-ideapad-5g](aarch64-laptops/lenovo-ideapad-5g), [lenovo-ideapad-5g-14q8x05](aarch64-laptops/lenovo-ideapad-5g-14q8x05) |
| Lenovo Miix 630 | [lenovo-miix-630](aarch64-laptops/lenovo-miix-630) |
| Lenovo Yoga C630 | [lenovo-yoga-c630](aarch64-laptops/lenovo-yoga-c630) |
| Microsoft Surface Pro X | [microsoft-surface-prox](aarch64-laptops/microsoft-surface-prox), [linux-surface/surface_pro_x_sq1](linux-surface/surface_pro_x_sq1), [linux-surface/surface_pro_x_sq2](linux-surface/surface_pro_x_sq2) |
| Samsung Galaxy Book2 | [samsung-galaxybook2](aarch64-laptops/samsung-galaxybook2) |
| Samsung Galaxy Book2 Go | [samsung-galaxybook2-go](aarch64-laptops/samsung-galaxybook2-go) |
| Samsung Galaxy Book Go | [samsung-galaxy-book-go](aarch64-laptops/samsung-galaxy-book-go) |
| Samsung Galaxy Book Go 5G | [samsung-galaxy-book-go-5g](aarch64-laptops/samsung-galaxy-book-go-5g) |
| Samsung Galaxy Book S | [samsung-galaxy-book-s](aarch64-laptops/samsung-galaxy-book-s) |
| TCL Book 14 Go | [tcl-book-14-go](aarch64-laptops/tcl-book-14-go) |
| Xiaomi Book S | [xiaomi-book-s](aarch64-laptops/xiaomi-book-s) |

## No dump yet

- Acer Aspire Go 15
- Acer Aspire Lite 14 AI
- Acer Swift Go 14 AI
- Acer Swift Spin 14 AI
- ASUS ProArt PZ13
- ASUS Vivobook 16
- Fujitsu FMV Note U
- HP OmniBook 3 14
- HP OmniBook 3 16 (16-ct0)
- HP OmniBook 5 16
- HP ProBook 4 G1q
- HP EliteBook X G2q
- HP OmniBook 5 14 (2026)
- HP OmniBook 5 16 (2026)
- HP OmniBook X 14 (14-kr0)
- HP OmniBook X 16 (16-cp0)
- HP OmniBook X 14 (2026)
- HP OmniBook X 16 (2026)
- HUMAIN Horizon
- Lenovo V15 G7 Q7X
- Lenovo ThinkPad T14s Gen 7
- Microsoft Surface Laptop 13-inch
- Samsung Galaxy Book4 Edge 15
- Samsung Galaxy Book5 Edge 5G
- Samsung Galaxy Book6 Edge 16

## Not matched to a model yet

- [hardware-reports/asustek-computer-inc/asus-vivobook-14-x1407qa-x1407qa](../reports/asustek-computer-inc/asus-vivobook-14-x1407qa-x1407qa)
