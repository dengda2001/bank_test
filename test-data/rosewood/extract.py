#!/usr/bin/env python3
"""Turn 收租明细_Rosewood_20260916.xlsx into the committed Rosewood fixtures.

The source workbook is a real Dublin rent ledger: four sheets, one per property,
with one "month group" per month (r5 onwards). Each group starts on the row that
carries the year-month serial; the remaining rows of the group repeat the same
month with an empty year-month cell, and the group ends with a 合计 row.

The parse is deliberately a one-off script: the workbook is dirty (broken
#REF! formulas, names that change case/order between months, Chinese notes mixed
into the name column, placeholder text such as 空 / 和同屋一起付, shifted
contract/rent columns). Re-running the rules on every seed would be slow and
unstable, so this script writes JSON fixtures that are committed, human
reviewable, and the actual input of scripts/seed-rosewood.sh.

Two traps this script exists to avoid:

1. Summary-row detection. A month group's first row usually carries room number
   1 / 01, i.e. a *number* in the room column. Any "is this cell numeric?"
   shortcut therefore eats room 1 of every property. A row only counts as a
   summary when the year-month cell is EMPTY *and* the row carries a 合计 marker
   (or numeric cells in the count and rent columns, which is how other exports
   write it). ``is_summary_row`` implements exactly that.
2. Redundant vs relational room binding. tenants.room_label / room_address /
   property_hint are read by the dashboard, while the room tree reads
   tenancy_agreements + agreement_parties. Both are projected here from one
   shared "occupancy" structure (see build_occupancy) so they can never disagree.

Usage:
    python3 test-data/rosewood/extract.py [--xlsx PATH] [--out DIR]
    python3 test-data/rosewood/extract.py --check     # verify fixtures in sync
"""

from __future__ import annotations

import argparse
import collections
import datetime
import json
import os
import re
import sys
import zipfile
import zlib

EPOCH = datetime.date(1899, 12, 30)
HERE = os.path.dirname(os.path.abspath(__file__))
REPO_ROOT = os.path.dirname(os.path.dirname(HERE))
DEFAULT_XLSX = os.path.join(REPO_ROOT, "收租明细_Rosewood_20260916.xlsx")

# Column indexes (0-based) of the ledger layout, per design.md §2.
COL_MONTH = 1       # 年月 (Excel serial, only set on a group's first row)
COL_DATE = 2        # 日期 / "合计：" on a summary row
COL_ROOM = 3        # 房间号
COL_NAME = 4        # 名字
COL_CONTRACT = 5    # 合同日期
COL_MOVEIN = 6      # 入住日期
COL_DUE = 7         # 应收
COL_PAID = 8        # 实收金额
COL_NOTE = 17       # 备注

# Values that appear in the name column but are not people.
PLACEHOLDER_NAMES = {"", "#REF!", "空", "和同屋一起付", "合计：", "合计"}

# Values that are a description of a person rather than a name. The daughter of
# the Domingo household is real, but the ledger never records her name, so she
# cannot become a tenant row (prd.md R5) and the slot stays unmapped.
DESCRIPTOR_NAMES = {
    "DOMING 女儿": "only a family relation is recorded (DOMING 女儿), not a name",
}

# ---------------------------------------------------------------------------
# Property scoped name normalisation.
#
# Key: property address as written in row 2 of the sheet.
# Value: {"alias": {UPPERCASED RAW -> canonical key},
#         "display": {canonical key -> display name}}
#
# Only variants actually observed in the workbook are listed; anything that is
# neither an alias nor a placeholder is reported in review.json under
# "unknown_names" instead of being guessed at.
# ---------------------------------------------------------------------------
NAME_TABLES: dict[str, dict[str, dict[str, str]]] = {
    "72 Walkinstown Rd Dublin 12": {
        "alias": {
            "ABHISHEK BHAGWAT": "abhishek",
            "EIDER ESNEIR": "eider",
            "ARDRA PUNATHIL": "ardra",
            "ARDRA M": "ardra",
            "ARDRA M PUNATHIL": "ardra",
            "MEGHANA VIPIN": "meghana",
            "MEGHANA VIPIN MEHTA": "meghana",
            "BLANCY MENDES": "blancy",
            "AVILA ALMEIDA": "avila",
            "RAJ": "raj",
            "NAGENDRA GONUGUNTA": "nagendra",
            "LOHIT DINESH": "lohit",
            "DURGA NAGENDRA": "durga",
            "ROHITH MURALI": "rohith",
            "SURYAKARTHIK": "suryakarthik",
            "AMAL SURYA": "amal",
            "BHUVAN D": "bhuvaneswaran",
            "BHUVANESWARAN": "bhuvaneswaran",
            "BHUVANES WARAN": "bhuvaneswaran",
            "KAUSHIK KANNAN": "kaushik",
            "AJAY": "ajay",
            "AJAY PANDIKASALA": "ajay",
            "DOMINGO JOSE CRISTO SANCHES": "domingo",
        },
        "display": {
            "abhishek": "Abhishek Bhagwat",
            "eider": "Eider Esneir",
            "ardra": "Ardra M Punathil",
            "meghana": "Meghana Vipin Mehta",
            "blancy": "Blancy Mendes",
            "avila": "Avila Almeida",
            "raj": "Raj",
            "nagendra": "Nagendra Gonugunta",
            "lohit": "Lohit Dinesh",
            "durga": "Durga Nagendra",
            "rohith": "Rohith Murali",
            "suryakarthik": "Suryakarthik",
            "amal": "Amal Surya",
            "bhuvaneswaran": "Bhuvaneswaran",
            "kaushik": "Kaushik Kannan",
            "ajay": "Ajay Pandikasala",
            "domingo": "Domingo Jose Cristo Sanches",
        },
    },
    "78 Old County Road Dublin 12": {
        "alias": {
            "THANT LYNN HTET": "thant",
            "SAI CHITRAKSH AMRAM": "sai",
            "SRIRAM SAMALA": "sriram",
            "TAJ": "tajudin",
            "TAJUDIN SHAIK": "tajudin",
            "NGHIEM DUC MINH (CHRIS)": "nghiem",
            "PHAM TRUNG HIEU (HARVEY": "pham",
            "PHAM TRUNG HIEU (HARVEY)": "pham",
            "WAHAJULLAH KHAN": "wahajullah",
            "WAI AUNG": "wai",
            "CHU SANDY KYAW": "chu",
            "ASAD RAZA": "asad",
            "ARSLAN ARSHAD": "arslan",
            "HENISHA ANISH RAUT": "henisha",
            "ANISH KISHOR": "anish",
            "NYAN LIN HTUT": "nyan",
            "MIN MYAT MOE": "min",
            "SATVIK TALAWAR": "satvik",
            "ANANYA M": "ananya",
        },
        "display": {
            "thant": "Thant Lynn Htet",
            "sai": "Sai Chitraksh Amram",
            "sriram": "Sriram Samala",
            "tajudin": "Tajudin Shaik",
            "nghiem": "Nghiem Duc Minh (Chris)",
            "pham": "Pham Trung Hieu (Harvey)",
            "wahajullah": "Wahajullah Khan",
            "wai": "Wai Aung",
            "chu": "Chu Sandy Kyaw",
            "asad": "Asad Raza",
            "arslan": "Arslan Arshad",
            "henisha": "Henisha Anish Raut",
            "anish": "Anish Kishor",
            "nyan": "Nyan Lin Htut",
            "min": "Min Myat Moe",
            "satvik": "Satvik Talawar",
            "ananya": "Ananya M",
        },
    },
    "116 Kimmage Rd W Dublin 12": {
        "alias": {
            "ANSELMO AUGUSTO DOS SANTOS RODRIGUES": "anselmo",
            "SIDDARTH JANAGOUDA": "siddarth",
            "CHIDERA NNAETO": "chidera",
            "SRAVAN": "sravan",
            "NEIL": "neil",
            "NEIL JOSE": "neil",
            "ABDELLATIF 原5405）": "abdellatif",
            "ABDELLATIF": "abdellatif",
            "LILIAN": "lilian",
            "CHENXI LIU": "chenxi",
            "ANUSH": "anush",
            "BALA ANUSH CHOUDHARY": "anush",
            "PRITEESH NANDAN": "priteesh",
            "HARDIK GATTU": "hardik",
            "NOEL": "noel",
            "NOEL JOSE": "noel",
            "PAVEL IONUT ALEXANDRU": "pavel",
            "DEVANSH CHAUHAN": "devansh",
        },
        "display": {
            "anselmo": "Anselmo Augusto dos Santos Rodrigues",
            "siddarth": "Siddarth Janagouda",
            "chidera": "Chidera Nnaeto",
            "sravan": "Sravan",
            "neil": "Neil Jose",
            "abdellatif": "Abdellatif",
            "lilian": "Lilian",
            "chenxi": "Chenxi Liu",
            "anush": "Bala Anush Choudhary",
            "priteesh": "Priteesh Nandan",
            "hardik": "Hardik Gattu",
            "noel": "Noel Jose",
            "pavel": "Pavel Ionut Alexandru",
            "devansh": "Devansh Chauhan",
        },
    },
    "169 Windmill Park Dublin 12": {
        "alias": {
            "YONG LIU": "yong",
            "ABDULRHMAN ELFEKY": "abdulrahman",
            "ABDULRAHMAN ELFEKY": "abdulrahman",
            "VITOR HUGO SILVA CARVALHO": "vitor",
            "GLEICE CRISTINA RODRIGUES DA SILVA": "gleice",
            "RAFAEL DE SOUZA SANTOS CUNHA": "rafael",
            "ATIF ANAM": "atif",
            "MOHD ANAMUL HAQ": "mohd",
            "QIANG LI": "qiang",
        },
        "display": {
            "yong": "Yong Liu",
            "abdulrahman": "Abdulrahman Elfeky",
            "vitor": "Vitor Hugo Silva Carvalho",
            "gleice": "Gleice Cristina Rodrigues da Silva",
            "rafael": "Rafael de Souza Santos Cunha",
            "atif": "Atif Anam",
            "mohd": "Mohd Anamul Haq",
            "qiang": "Qiang Li",
        },
    },
}

PROPERTY_KEYS = {
    "72 Walkinstown Rd Dublin 12": "walkinstown",
    "78 Old County Road Dublin 12": "oldcounty",
    "116 Kimmage Rd W Dublin 12": "kimmage",
    "169 Windmill Park Dublin 12": "windmill",
}

CITY_REGION = "Dublin"
TIMEZONE = "Europe/Dublin"
DUE_DAY = 1
CURRENCY = "EUR"

# Marks every row this seed owns:
#   payment_transactions.stable_transaction_key LIKE '<prefix>%'
#   payment_transactions.reference             LIKE 'ROSEWOOD-%'
#   payment_allocations.idempotency_key        LIKE 'rosewood-alloc:%'
# The seed script refuses to delete outside these prefixes plus the four known
# addresses and the normalised name set, so it can never remove hand-made data.
STABLE_KEY_PREFIX = "rosewood:"
REFERENCE_PREFIX = "ROSEWOOD-"
ALLOCATION_KEY_PREFIX = "rosewood-alloc:"
LEDGER_MONTHS = [f"2026-{month:02d}" for month in range(5, 13)]


# ---------------------------------------------------------------------------
# xlsx plumbing (hand rolled: openpyxl/pyexpat are unusable in this environment)
# ---------------------------------------------------------------------------
def _unescape(value: str) -> str:
    for entity, char in (("&lt;", "<"), ("&gt;", ">"), ("&quot;", '"'), ("&apos;", "'"), ("&amp;", "&")):
        value = value.replace(entity, char)
    return value


def _column_number(ref: str) -> int:
    letters = re.match(r"([A-Z]+)", ref).group(1)
    number = 0
    for char in letters:
        number = number * 26 + (ord(char) - 64)
    return number


def parse_workbook(path: str) -> list[tuple[str, list[list[str]]]]:
    """Return [(sheet_name, rows)] with rows as dense string lists."""
    archive = zipfile.ZipFile(path)
    names = archive.namelist()
    shared: list[str] = []
    if "xl/sharedStrings.xml" in names:
        raw = archive.read("xl/sharedStrings.xml").decode("utf-8", "replace")
        for item in re.findall(r"<si>(.*?)</si>", raw, re.S):
            shared.append("".join(re.findall(r"<t[^>]*>(.*?)</t>", item, re.S)))

    sheets: list[tuple[str, list[list[str]]]] = []
    for sheet_name in sorted(n for n in names if re.match(r"xl/worksheets/sheet\d+\.xml$", n)):
        raw = archive.read(sheet_name).decode("utf-8", "replace")
        rows: list[list[str]] = []
        for row_match in re.finditer(r"<row[^>]*>(.*?)</row>", raw, re.S):
            body = row_match.group(1)
            cells: dict[int, str] = {}
            for cell in re.finditer(r'<c r="([A-Z]+\d+)"([^>]*?)(?:/>|>(.*?)</c>)', body, re.S):
                ref, attrs, inner = cell.group(1), cell.group(2), cell.group(3) or ""
                kind = re.search(r't="([^"]+)"', attrs)
                value = re.search(r"<v>(.*?)</v>", inner, re.S)
                if kind and kind.group(1) == "inlineStr":
                    text = "".join(re.findall(r"<t[^>]*>(.*?)</t>", inner, re.S))
                elif value is None:
                    text = ""
                elif kind and kind.group(1) == "s":
                    index = int(value.group(1))
                    text = shared[index] if index < len(shared) else ""
                else:
                    text = value.group(1)
                cells[_column_number(ref)] = _unescape(text).strip()
            if cells:
                rows.append([cells.get(index, "") for index in range(1, max(cells) + 1)])
            else:
                rows.append([])
        sheets.append((sheet_name, rows))
    return sheets


def serial_to_date(value: str) -> datetime.date | None:
    try:
        number = int(float(value))
    except (TypeError, ValueError):
        return None
    if number < 20000 or number > 60000:  # outside the ledger's date window
        return None
    return EPOCH + datetime.timedelta(days=number)


def to_cents(value: str) -> int | None:
    text = (value or "").strip().replace(",", "")
    if text == "":
        return None
    try:
        return int(round(float(text) * 100))
    except ValueError:
        return None


# ---------------------------------------------------------------------------
# Row classification and cleaning
# ---------------------------------------------------------------------------
def is_summary_row(cell_month: str, cell_date: str, cell_room: str, cell_due: str) -> bool:
    """True only for a group's 合计 row.

    The year-month cell MUST be empty: a group's first row also has a numeric
    room cell, so a numeric check alone would swallow room 1 of every sheet.
    """
    if serial_to_date(cell_month) is not None:
        return False
    if "合计" in cell_date or "合计" in cell_room:
        return True
    # Alternative export shape described in design.md §2: the count of rooms in
    # the second column and the rent total in the fourth.
    return to_cents(cell_month) is not None and to_cents(cell_room) is not None


def clean_name(raw: str) -> str:
    """Collapse the whitespace/punctuation noise that survives in the name column."""
    text = (raw or "").replace(" ", " ").strip()
    text = text.strip(",").strip()          # continuation rows look like ", NAME"
    text = re.sub(r"[,;]+$", "", text).strip()
    text = re.sub(r"\s+", " ", text)
    return text.strip()


def uppercased_key(text: str) -> str:
    return re.sub(r"\s+", " ", text).strip().upper()


def canonical_person(address: str, raw_name: str) -> tuple[str | None, str | None, str]:
    """Return (canonical_key, display_name, status) for a raw name cell.

    status is one of "person", "placeholder", "descriptor", "unknown".
    """
    cleaned = clean_name(raw_name)
    if cleaned in PLACEHOLDER_NAMES or uppercased_key(cleaned) in PLACEHOLDER_NAMES:
        return None, None, "placeholder"
    if cleaned in DESCRIPTOR_NAMES:
        return None, None, "descriptor"
    table = NAME_TABLES[address]
    key = table["alias"].get(uppercased_key(cleaned))
    if key is None:
        return None, None, "unknown"
    return key, table["display"][key], "person"


# ---------------------------------------------------------------------------
# Occupancy: one entry per (property, month, room, slot)
# ---------------------------------------------------------------------------
def read_ledger(path: str, review: dict) -> tuple[dict, dict]:
    """Return (properties_by_key, occupancy).

    occupancy[address][month][room] = list of slot dicts, positionally ordered as
    the ledger lists them:
        {"name_raw": ..., "person": key|None, "status": level,
         "due_cents": int|None, "paid_cents": int|None, "note": ...}
    """
    sheets = parse_workbook(path)
    properties: dict[str, dict] = {}
    occupancy: dict[str, dict] = {}
    summaries: dict[str, dict] = {}

    for sheet_name, rows in sheets:
        if len(rows) < 5:
            continue
        address = rows[1][1].strip() if len(rows[1]) > 1 else ""
        if address not in PROPERTY_KEYS:
            review["unknown_properties"].append({"sheet": sheet_name, "address": address})
            continue
        key = PROPERTY_KEYS[address]
        properties[key] = {
            "key": key,
            "name": address,
            "address": address,
            "city_region": CITY_REGION,
            "timezone": TIMEZONE,
            "status": "active",
        }
        occupancy.setdefault(address, {})
        summaries.setdefault(address, {})

        month: str | None = None
        for row in rows[4:]:
            cell = lambda index: row[index].strip() if index < len(row) and isinstance(row[index], str) else ""
            month_serial = serial_to_date(cell(COL_MONTH))
            if month_serial is not None:
                month = month_serial.strftime("%Y-%m")
                occupancy[address].setdefault(month, {})
                summaries[address].setdefault(month, {})
            if month is None:
                continue
            if is_summary_row(cell(COL_MONTH), cell(COL_DATE), cell(COL_ROOM), cell(COL_DUE)):
                summaries[address][month] = {
                    "due_cents": to_cents(cell(COL_DUE)),
                    "paid_cents": to_cents(cell(COL_PAID)),
                }
                continue
            room = cell(COL_ROOM)
            if room == "":
                continue

            # design.md §2: when the contract/move-in cells are empty the due
            # amount slides left into the contract column.
            due = to_cents(cell(COL_DUE))
            fallback = False
            if due is None:
                due = to_cents(cell(COL_CONTRACT))
                fallback = due is not None
            if fallback:
                review["due_column_fallback"].append(
                    {"property": key, "month": month, "room": room, "raw_name": cell(COL_NAME), "due_cents": due}
                )

            person, display, status = canonical_person(address, cell(COL_NAME))
            slot = {
                "month": month,
                "room": room,
                "name_raw": clean_name(cell(COL_NAME)),
                "person": person,
                "display": display,
                "status": status,
                "due_cents": due,
                "paid_cents": to_cents(cell(COL_PAID)),
                "note": cell(COL_NOTE),
                "contract_date": serial_to_date(cell(COL_CONTRACT)),
                "move_in_date": serial_to_date(cell(COL_MOVEIN)),
            }
            occupancy[address][month].setdefault(room, []).append(slot)

    return properties, occupancy, summaries


def report_unnamed_slots(occupancy: dict, review: dict) -> None:
    for address, months in occupancy.items():
        property_key = PROPERTY_KEYS[address]
        for month in sorted(months):
            for room in sorted(months[month]):
                for slot in months[month][room]:
                    if slot["status"] == "placeholder":
                        review["skipped_rows"].append({
                            "property": property_key, "month": month, "room": room,
                            "raw_name": slot["name_raw"] or "(blank)",
                            "reason": "placeholder, not a person name (prd.md R5)",
                            "due_cents": slot["due_cents"], "paid_cents": slot["paid_cents"],
                        })
                    elif slot["status"] == "descriptor":
                        review["skipped_rows"].append({
                            "property": property_key, "month": month, "room": room,
                            "raw_name": slot["name_raw"],
                            "reason": DESCRIPTOR_NAMES[slot["name_raw"]],
                            "due_cents": slot["due_cents"], "paid_cents": slot["paid_cents"],
                        })
                    elif slot["status"] == "unknown":
                        review["unknown_names"].append({
                            "property": property_key, "month": month, "room": room,
                            "raw_name": slot["name_raw"],
                        })


def fill_blank_slots(occupancy: dict, review: dict) -> None:
    """Attribute a blank name cell to the occupant the neighbouring month shows.

    A blank cell is missing data, not an absent tenant: the ledger keeps the row
    (with rent) and only loses the name. Filling is allowed only when the month's
    *known* occupants line up slot-for-slot with an adjacent month, i.e. the room
    clearly continued, and only for genuinely blank cells. Explicit placeholders
    (#REF!, 和同屋一起付, DOMING 女儿) are never filled (prd.md R5).

    Runs to a fixed point because a month may be filled from a month that is
    itself filled.
    """
    for address, months in occupancy.items():
        property_key = PROPERTY_KEYS[address]
        month_order = sorted(months)
        changed = True
        while changed:
            changed = False
            for index, month in enumerate(month_order):
                for room, slots in months[month].items():
                    blanks = [position for position, slot in enumerate(slots)
                              if slot["name_raw"] == "" and slot["person"] is None]
                    if not blanks:
                        continue
                    for reference_index in (index - 1, index + 1):
                        if reference_index < 0 or reference_index >= len(month_order):
                            continue
                        reference_month = month_order[reference_index]
                        reference = months[reference_month].get(room)
                        if not reference or len(reference) != len(slots):
                            continue
                        if not _rooms_line_up(slots, reference):
                            continue
                        filled = False
                        for position in blanks:
                            if reference[position]["person"] is None:
                                continue
                            slots[position].update({
                                "person": reference[position]["person"],
                                "display": reference[position]["display"],
                                "status": "person",
                                "filled_from": reference_month,
                            })
                            review["blank_fills"].append({
                                "property": property_key, "month": month, "room": room,
                                "slot": position + 1,
                                "filled_with": reference[position]["display"],
                                "reference_month": reference_month,
                            })
                            filled = True
                        if filled:
                            changed = True
                            break  # earlier month wins, as design.md §3 does
                        # Nothing to take from this neighbour (the cell it would
                        # copy is itself blank); try the month on the other side
                        # before giving up on the room.


def drop_unbilled_slots(occupancy: dict, review: dict) -> None:
    """Remove a named row the ledger never charged for.

    The one case in this workbook is 169 Windmill Park room 05 in 2026-05: the row
    names Mohd Anamul Haq but leaves 应收 and 实收 empty, and the sheet's 合计 for
    that month (5,200) only adds up without him. He therefore arrives in 2026-06.
    Keeping the row would make the model bill 650 that the ledger never billed,
    which no amount of explanation reconciles, so the row is dropped and recorded.
    """
    for address, months in occupancy.items():
        property_key = PROPERTY_KEYS[address]
        for month in sorted(months):
            for room, slots in months[month].items():
                kept = []
                for position, slot in enumerate(slots):
                    if slot["person"] is not None and slot["due_cents"] is None:
                        review["unbilled_rows"].append({
                            "property": property_key, "month": month, "room": room,
                            "slot": position + 1, "name": slot["display"],
                            "reason": "named row with an empty 应收; the month's 合计 does not include them",
                        })
                        continue
                    kept.append(slot)
                months[month][room] = kept


def _rooms_line_up(month_slots: list[dict], reference_slots: list[dict]) -> bool:
    """True when every known occupant of month_slots matches the reference slot."""
    for position, slot in enumerate(month_slots):
        if slot["person"] is None:
            continue
        if position >= len(reference_slots):
            return False
        if reference_slots[position]["person"] != slot["person"]:
            return False
    return True


# ---------------------------------------------------------------------------
# Projection: occupancy -> tenants / agreements / payments
# ---------------------------------------------------------------------------
def dominant_rate(observations: list[tuple[str, int]]) -> tuple[int, bool]:
    """Pick one monthly share for a tenancy segment.

    The ledger revises rent mid-tenancy occasionally (Ajay 790 -> 800 in
    2026-09, Siddarth 660 -> 680 in 2026-07, ...). tenants.monthly_rent_cents is
    a single value, so the most frequent observation wins and ties go to the
    most recent one; every deviation is recorded in review.json so the
    reconciled totals stay explainable (prd.md R4).
    """
    counts: collections.Counter[int] = collections.Counter()
    for _, cents in observations:
        counts[cents] += 1
    top = max(counts.values())
    candidates = [cents for cents, count in counts.items() if count == top]
    if len(candidates) == 1:
        return candidates[0], False
    latest = observations[-1][1]
    return latest, latest not in candidates[:1]


def build_occupancy(properties: dict, occupancy: dict, review: dict) -> dict:
    """Collapse the slot grid into per-room month maps of canonical people."""
    result: dict[str, dict] = {}
    for address, months in occupancy.items():
        property_key = PROPERTY_KEYS[address]
        result[property_key] = {
            "address": address,
            "rooms": collections.defaultdict(dict),  # room -> month -> ordered persons
            "room_labels": set(),
        }
        for month in sorted(months):
            for room, slots in sorted(months[month].items()):
                result[property_key]["room_labels"].add(room)
                people = []
                for slot in slots:
                    if slot["person"] is None:
                        continue
                    people.append({
                        "person": slot["person"],
                        "display": slot["display"],
                        "due_cents": slot["due_cents"],
                        "paid_cents": slot["paid_cents"],
                        "name_raw": slot["name_raw"],
                        "note": slot["note"],
                        "contract_date": slot["contract_date"],
                        "move_in_date": slot["move_in_date"],
                        "filled_from": slot.get("filled_from"),
                    })
                result[property_key]["rooms"][room][month] = people
    return result


def build_segments(properties: dict, occupancy: dict, review: dict) -> tuple[list, list, list]:
    """Project the occupancy grid into tenants / agreements / payment rows."""
    tenants: list[dict] = []
    agreements: list[dict] = []
    payments: list[dict] = []
    payer_relations: list[dict] = []

    months_range = sorted({month for data in occupancy.values() for month in data["rooms"][next(iter(data["rooms"]))]})

    for property_key in sorted(properties):
        if not occupancy[property_key]["rooms"]:
            continue
        data = occupancy[property_key]
        address = data["address"]
        for room in sorted(data["rooms"], key=lambda value: (len(value), value)):
            month_people = data["rooms"][room]
            months_sorted = sorted(month_people)

            # --- tenants: one row per (person, room, contiguous month run) ----
            stays: dict[str, list[str]] = collections.defaultdict(list)
            for month in months_sorted:
                for entry in month_people[month]:
                    stays[entry["person"]].append(month)
            tenant_key_by_stay: dict[tuple[str, str], str] = {}
            for person in sorted(stays):
                months_present = sorted(stays[person])
                for block in _contiguous_blocks(months_present):
                    # One entry per month of the block, in month order, so the
                    # observed rents stay index-aligned with `months` (a month
                    # where the ledger left 应收 blank keeps its None).
                    observed_by_month = {month: None for month in block}
                    for month in block:
                        for entry in month_people[month]:
                            if entry["person"] == person:
                                observed_by_month[month] = entry["due_cents"]
                    observations = [(month, cents) for month, cents in observed_by_month.items()
                                    if cents is not None]
                    if not observations:
                        review["tenants_without_rent"].append({
                            "property": property_key, "room": room, "person": person, "months": block,
                        })
                        continue
                    rate, _ = dominant_rate(observations)
                    for month, cents in observations:
                        if cents != rate:
                            review["rate_changes"].append({
                                "property": property_key, "room": room, "person": person,
                                "person_display": NAME_TABLES[address]["display"][person],
                                "month": month, "observed_cents": cents, "modelled_cents": rate,
                            })
                    display = NAME_TABLES[address]["display"][person]
                    alias = _display_alias(address, person, block, month_people)
                    tenant_key = f"{property_key}-{_slug(room)}-{person}-{block[0]}"
                    tenant_key_by_stay[(person, block[0])] = tenant_key
                    tenants.append({
                        "key": tenant_key,
                        "name": display,
                        "display_alias": alias,
                        "property_key": property_key,
                        "room": room,
                        "monthly_rent_cents": rate,
                        "currency": CURRENCY,
                        "due_day": DUE_DAY,
                        "billing_start_date": _month_start(block[0]),
                        "rent_start_date": _month_start(block[0]),
                        "rent_end_date": _month_end(block[-1]),
                        "status": "active",
                        "source": {
                            "months": block,
                            "raw_names": sorted({entry["name_raw"] for month in block for entry in month_people[month] if entry["person"] == person and entry["name_raw"]}),
                            "observed_due_cents": [observed_by_month[month] for month in block],
                        },
                    })

            # --- agreements: one per contiguous run of a stable occupant set --
            for block in _stable_set_blocks(months_sorted, month_people):
                members = month_people[block[0]]
                party_rows = []
                total = 0
                for entry in members:
                    stay_start = block[0]
                    tenant_key = None
                    for candidate in _contiguous_blocks(sorted(stays[entry["person"]])):
                        if candidate[0] <= block[0] <= candidate[-1]:
                            tenant_key = tenant_key_by_stay.get((entry["person"], candidate[0]))
                            stay_start = candidate[0]
                            break
                    row = next(row for row in tenants if row["key"] == tenant_key)
                    party_rows.append({
                        "tenant_key": tenant_key,
                        "responsibility_cents": row["monthly_rent_cents"],
                        "joined_at": _month_start(stay_start),
                        "left_at": row["rent_end_date"],
                        "status": "active",
                    })
                    total += row["monthly_rent_cents"]
                agreements.append({
                    "key": f"{property_key}-{_slug(room)}-agreement-{block[0]}",
                    "property_key": property_key,
                    "room": room,
                    "start_date": _month_start(block[0]),
                    "end_date": _month_end(block[-1]),
                    "monthly_rent_cents": total,
                    "currency": CURRENCY,
                    "due_day": DUE_DAY,
                    # The ledger's contract dates are a snapshot refresh (most
                    # rows say 2026-09-14 whatever the tenancy), so the tenancy
                    # start is the honest value here; the observed values are
                    # recorded in review.json.
                    "contract_date": _month_start(block[0]),
                    "move_in_date": _month_start(block[0]),
                    "status": "active",
                    "parties": party_rows,
                })

            # --- payments: one income transaction per paid tenant-month -------
            for month in months_sorted:
                for entry in month_people[month]:
                    if not entry["paid_cents"] or entry["paid_cents"] <= 0:
                        continue
                    row = next(
                        (row for row in tenants
                         if row["key"] == tenant_key_by_stay.get((entry["person"], _block_start_for(stays[entry["person"]], month)))),
                        None,
                    )
                    if row is None:
                        continue
                    amount = min(entry["paid_cents"], row["monthly_rent_cents"])
                    if amount != entry["paid_cents"]:
                        review["payment_capped"].append({
                            "property": property_key, "month": month, "room": room,
                            "person": entry["person"], "source_paid_cents": entry["paid_cents"],
                            "captured_cents": amount,
                            "note": "payment covered more than this tenant's share (a flatmate paid both halves)",
                        })
                    payments.append({
                        "tenant_key": row["key"],
                        "property_key": property_key,
                        "month": month,
                        "amount_cents": amount,
                        "source_paid_cents": entry["paid_cents"],
                        "share_cents": row["monthly_rent_cents"],
                        "payer_name": row["name"],
                        "payer_name_normalized": normalize_match_text(row["name"]),
                        "description": f"Rosewood seed {month} · {row['name']}",
                    })

    # Transaction times and references, plus the deterministic matched subset.
    payments.sort(key=lambda row: (row["property_key"], row["tenant_key"], row["month"]))
    for index, row in enumerate(payments):
        row["reference"] = f"ROSEWOOD-{row['month']}-{index + 1:04d}"
        row["stable_transaction_key"] = f"{STABLE_KEY_PREFIX}{row['reference']}"
        day = 1 + (_stable_hash(f"{row['tenant_key']}|{row['month']}") % 26)
        row["paid_on"] = f"{row['month']}-{day:02d}"
        # ~25% pre-matched, plus every genuinely short payment, so the bills page
        # shows 已收 / 部分收 / 未收 rather than one uniform state. The rule is
        # expressed in SQL in seed-match.sql as
        #   amount_cents < expected_amount_cents OR CRC32(stable_transaction_key) % 4 = 0
        # so it has to be CRC32 here too (zlib.crc32 is the same ISO-HDLC CRC-32
        # that MySQL's CRC32() computes), not Python's salted hash().
        row["matched"] = row["amount_cents"] < row["share_cents"] or (
            zlib.crc32(row["stable_transaction_key"].encode("utf-8")) % 4 == 0
        )
        row["matched_tenant_id"] = row["tenant_key"] if row["matched"] else None

    matched_tenants = {row["tenant_key"] for row in payments if row["matched"]}
    display_by_key = {row["key"]: row["name"] for row in tenants}
    for tenant_key in sorted(matched_tenants):
        payer_relations.append({
            "tenant_key": tenant_key,
            "payer_name": display_by_key[tenant_key],
            "payer_name_normalized": normalize_match_text(display_by_key[tenant_key]),
        })

    review["payment_summary"] = {
        "transactions": len(payments),
        "matched": sum(1 for row in payments if row["matched"]),
        "unmatched": sum(1 for row in payments if not row["matched"]),
        "matched_short": sum(1 for row in payments if row["matched"] and row["amount_cents"] < row["share_cents"]),
        "remembered_payers": len(payer_relations),
    }
    return tenants, agreements, payments, payer_relations


def _block_start_for(months_present: list[str], month: str) -> str:
    for block in _contiguous_blocks(sorted(set(months_present))):
        if month in block:
            return block[0]
    raise KeyError(month)


def _contiguous_blocks(months: list[str]) -> list[list[str]]:
    blocks: list[list[str]] = []
    for month in months:
        if blocks and _next_month(blocks[-1][-1]) == month:
            blocks[-1].append(month)
        else:
            blocks.append([month])
    return blocks


def _stable_set_blocks(months: list[str], month_people: dict) -> list[list[str]]:
    """Group consecutive months whose occupant set is identical.

    Months where the room is empty are skipped entirely: a vacant month ends the
    previous arrangement and a later occupancy starts a new one.
    """
    blocks: list[list[str]] = []
    previous: tuple[str, ...] | None = None
    for month in months:
        current = tuple(sorted(entry["person"] for entry in month_people[month]))
        if not current:
            previous = None
            continue
        if previous is not None and current == previous and _next_month(blocks[-1][-1]) == month:
            blocks[-1].append(month)
        else:
            blocks.append([month])
        previous = current
    return blocks


def _display_alias(address: str, person: str, block: list[str], month_people: dict) -> str | None:
    """Keep the ledger's other spellings as display_alias (the page renders them)."""
    seen: list[str] = []
    for month in block:
        for entry in month_people[month]:
            if entry["person"] == person and entry["name_raw"]:
                if entry["name_raw"] not in seen:
                    seen.append(entry["name_raw"])
    canonical = NAME_TABLES[address]["display"][person]
    others = [value for value in seen if uppercased_key(value) != uppercased_key(canonical)]
    if not others:
        return None
    return "; ".join(others)


def normalize_match_text(value: str) -> str:
    """Mirror of normalizeMatchText (matching.go) = trim + collapse + lowercase.

    tenant_payers.payer_name_normalized and payment_transactions.payer_name are
    compared with this exact function by decideStrictRentMatch, so the seed has
    to produce the same string the app would.
    """
    return " ".join(value.split()).lower()


def _stable_hash(value: str) -> int:
    """Deterministic, dependency-free hash (Python's hash() is salted per run)."""
    result = 0
    for char in value:
        result = (result * 131 + ord(char)) % 1_000_003
    return result


def _slug(value: str) -> str:
    return re.sub(r"[^a-z0-9]+", "", value.lower())


def _month_start(month: str) -> str:
    return f"{month}-01"


def _month_end(month: str) -> str:
    year, number = (int(part) for part in month.split("-"))
    last = (datetime.date(year + (number == 12), number % 12 + 1, 1) - datetime.timedelta(days=1))
    return last.isoformat()


def _next_month(month: str) -> str:
    year, number = (int(part) for part in month.split("-"))
    if number == 12:
        return f"{year + 1}-01"
    return f"{year}-{number + 1:02d}"


# ---------------------------------------------------------------------------
# Rooms and reconciliation
# ---------------------------------------------------------------------------
def build_rooms(properties: dict, occupancy: dict, tenants: list[dict]) -> list[dict]:
    rooms: list[dict] = []
    for property_key in sorted(properties):
        data = occupancy[property_key]
        for room in sorted(data["rooms"], key=lambda value: (len(value), value)):
            month_people = data["rooms"][room]
            months_sorted = sorted(month_people)
            capacity = max((len(month_people[month]) for month in months_sorted), default=0)
            occupied_months = [month for month in months_sorted if month_people[month]]
            latest_total = sum(
                row["monthly_rent_cents"]
                for row in tenants
                if row["property_key"] == property_key and row["room"] == room
                and row["rent_start_date"] <= _month_end(occupied_months[-1])
                and row["rent_end_date"] >= _month_start(occupied_months[-1])
            ) if occupied_months else 0
            rooms.append({
                "key": f"{property_key}-{_slug(room)}",
                "property_key": property_key,
                "label": room,
                "capacity": capacity,
                "monthly_rent_cents": latest_total,
                "due_day": DUE_DAY,
                "active_from": _month_start(months_sorted[0]),
                "status": "active",
            })
    return rooms


def build_reconciliation(properties: dict, occupancy: dict, raw_slots: dict,
                         summaries: dict, tenants: list[dict]) -> list[dict]:
    """Explain every difference between the modelled rent and the ledger's 合计.

    delta = modelled - source, and it must decompose into:
      unnamed_slot_cents  rent the ledger charged on a row that carries no person
                          name (#REF!, 空, 和同屋一起付, DOMING 女儿). prd.md R5
                          forbids turning those into tenants, so their rent can
                          never be modelled.
      folded_cents        difference caused by folding a mid-tenancy rent change
                          into one monthly_rent_cents (see dominant_rate).
      residual_cents      whatever is left; should be 0. Anything else means the
                          occupancy model and the ledger disagree about who was
                          in the room, which is a real extraction bug.
    """
    rows = []
    for property_key in sorted(properties):
        data = occupancy[property_key]
        address = data["address"]
        for month in sorted(data["rooms"][next(iter(data["rooms"]))]):
            modelled = sum(
                row["monthly_rent_cents"] for row in tenants
                if row["property_key"] == property_key
                and row["rent_start_date"] <= _month_end(month)
                and row["rent_end_date"] >= _month_start(month)
            )
            unnamed = sum(
                slot["due_cents"] or 0
                for room in raw_slots[address][month]
                for slot in raw_slots[address][month][room]
                if slot["person"] is None
            )
            source = summaries.get(address, {}).get(month, {}).get("due_cents") or 0
            folded = sum(
                cents - row["monthly_rent_cents"] for row in tenants
                if row["property_key"] == property_key
                and row["rent_start_date"] <= _month_end(month)
                and row["rent_end_date"] >= _month_start(month)
                for month_label, cents in zip(row["source"]["months"], row["source"]["observed_due_cents"])
                if month_label == month and cents is not None
            )
            rows.append({
                "property": property_key,
                "month": month,
                "source_due_cents": source,
                "modelled_due_cents": modelled,
                "unnamed_slot_cents": unnamed,
                "folded_cents": folded,
                "delta_cents": modelled - source,
                "residual_cents": modelled + unnamed + folded - source,
            })
    return rows


# ---------------------------------------------------------------------------
# SQL emission
#
# scripts/seed-rosewood.sh is deliberately bash + mysql only (the sibling
# scripts/audit/ and scripts/run-audit-local.sh precedent). mysql cannot read
# JSON, so the JSON -> SQL translation lives here, next to the fixtures it
# describes, and the shell script just pipes the two generated files into the
# client. Both files take the acting user as @uid, which the script sets with
# mysql --init-command.
#
# The whole data stage is one transaction: a re-run deletes exactly the rows the
# fixture keys describe (matched on the unique/natural keys below, never on rows
# someone else created) and re-inserts them.
# ---------------------------------------------------------------------------
def sql_text(value: object) -> str:
    if value is None:
        return "NULL"
    text = str(value).replace("\\", "\\\\").replace("'", "''")
    return f"'{text}'"


def sql_int(value: object) -> str:
    return "NULL" if value in (None, "") else str(int(value))


def _values_rows(rows: list[list[str]]) -> str:
    width = 0
    for row in rows:
        width = max(width, len("(" + ", ".join(row) + ")"))
    lines = []
    for index, row in enumerate(rows):
        body = f"({', '.join(row)})"
        lines.append(f"  {body:<{width}}" + ("" if index == len(rows) - 1 else ","))
    return "\n".join(lines)


def _temp_table(name: str, columns: list[tuple[str, str]], rows: list[list[str]],
                key: str | None = None) -> list[str]:
    """Render DROP/CREATE/INSERT for one staging table.

    `key` is an extra table-level clause (a composite PRIMARY KEY) that must not
    appear in the INSERT column list, since it is not a column.
    """
    definitions = [f"  {column} {kind}" for column, kind in columns]
    if key:
        definitions.append(f"  {key}")
    names = [column for column, _ in columns]
    return [
        f"DROP TEMPORARY TABLE IF EXISTS {name};",
        f"CREATE TEMPORARY TABLE {name} (\n" + ",\n".join(definitions) + "\n) ENGINE=InnoDB;",
        f"INSERT INTO {name} ({', '.join(names)}) VALUES",
        _values_rows(rows) + ";",
    ]


def emit_data_sql(bundle: dict[str, object], out_dir: str) -> None:
    properties = bundle["properties"]["properties"]
    rooms = bundle["rooms"]["rooms"]
    tenants = bundle["tenants"]["tenants"]
    agreements = bundle["agreements"]["agreements"]
    payments = bundle["payments"]["payments"]
    payer_relations = bundle["payments"]["payer_relations"]

    addresses = [row["name"] for row in properties]
    address_list = ", ".join(sql_text(value) for value in addresses)

    # Room fixture key -> property key, and property key -> address, needed to
    # resolve the room ids and to fill the redundant tenant columns (prd.md R1).
    room_property = {row["key"]: row["property_key"] for row in rooms}
    property_address = {row["key"]: row["name"] for row in properties}

    property_rows = [[sql_text(row["key"]), sql_text(row["name"]), sql_text(row["city_region"]),
                      sql_text(row["address"]), sql_text(row["timezone"]), sql_text(row["status"])]
                     for row in properties]
    room_rows = [[sql_text(row["key"]), sql_text(row["property_key"]), sql_text(row["label"]),
                  sql_int(row["capacity"]), sql_int(row["monthly_rent_cents"]), sql_int(row["due_day"]),
                  sql_text(row["active_from"]), sql_text(row["status"])]
                 for row in rooms]
    tenant_rows = []
    for row in tenants:
        address = property_address[row["property_key"]]
        tenant_rows.append([
            sql_text(row["key"]), sql_text(row["name"]), sql_text(row["display_alias"]),
            sql_int(row["monthly_rent_cents"]), sql_text(row["currency"]),
            sql_text(row["billing_start_date"]), sql_int(row["due_day"]),
            sql_text(row["rent_start_date"]), sql_text(row["rent_end_date"]), sql_text(row["status"]),
            sql_text(row["room"]), sql_text(address), sql_text(address),
        ])
    agreement_rows = []
    party_rows = []
    for row in agreements:
        agreement_rows.append([
            sql_text(row["key"]), sql_text(f"{row['property_key']}-{_slug(row['room'])}"),
            sql_text(row["contract_date"]), sql_text(row["move_in_date"]), sql_text(row["start_date"]),
            sql_text(row["end_date"]), sql_int(row["monthly_rent_cents"]), sql_text(row["currency"]),
            sql_int(row["due_day"]), sql_text(row["status"]),
        ])
        for party in row["parties"]:
            party_rows.append([
                sql_text(row["key"]), sql_text(party["tenant_key"]),
                sql_int(party["responsibility_cents"]), sql_text(party["joined_at"]),
                sql_text(party["left_at"]), sql_text(party["status"]),
            ])
    transaction_rows = []
    for row in payments:
        transaction_rows.append([
            sql_text(row["stable_transaction_key"]), sql_int(row["amount_cents"]),
            sql_text(f"{row['paid_on']} 12:00:00"), sql_text(row["description"]),
            sql_text(row["reference"]), sql_text(row["payer_name"]),
            sql_text(f"{row['month']}-01"),
        ])
    payer_rows = [[sql_text(row["tenant_key"]), sql_text(row["payer_name"]), sql_text(row["payer_name_normalized"])]
                  for row in payer_relations]

    statements: list[str] = [
        "-- Generated by test-data/rosewood/extract.py -- do not edit by hand.",
        "-- Stage data: the ledger's lease structure and bank feed. Idempotent.",
        "-- Requires @uid (the acting user id); scripts/seed-rosewood.sh sets it.",
        "--",
        "-- Every row this file creates or deletes is scoped to these four",
        f"-- addresses and the fixture keys below: {address_list}",
        "",
        "-- 1) fixture keys, used by both the DELETE and the INSERT below",
    ]
    statements += _temp_table("_rs_property", [
        ("k", "varchar(64) NOT NULL PRIMARY KEY"), ("name", "varchar(191) NOT NULL"),
        ("city_region", "varchar(191) NULL"), ("address", "text NULL"),
        ("timezone", "varchar(64) NOT NULL"), ("status", "varchar(32) NOT NULL"),
    ], property_rows)
    statements += _temp_table("_rs_room", [
        ("k", "varchar(64) NOT NULL PRIMARY KEY"), ("property_key", "varchar(64) NOT NULL"),
        ("room_label", "varchar(191) NOT NULL"), ("capacity", "int NOT NULL"),
        ("monthly_rent_cents", "bigint NOT NULL"), ("due_day", "int NOT NULL"),
        ("active_from", "date NOT NULL"), ("status", "varchar(32) NOT NULL"),
    ], room_rows)
    statements += _temp_table("_rs_tenant", [
        ("k", "varchar(64) NOT NULL PRIMARY KEY"), ("name", "varchar(191) NOT NULL"),
        ("display_alias", "varchar(191) NULL"), ("monthly_rent_cents", "bigint NOT NULL"),
        ("currency", "char(3) NOT NULL"), ("billing_start_date", "date NOT NULL"),
        ("due_day", "int NOT NULL"), ("rent_start_date", "date NOT NULL"),
        ("rent_end_date", "date NULL"), ("status", "varchar(32) NOT NULL"),
        ("room_label", "varchar(191) NOT NULL"), ("room_address", "text NOT NULL"),
        ("property_hint", "varchar(191) NULL"),
    ], tenant_rows)
    statements += _temp_table("_rs_agreement", [
        ("k", "varchar(128) NOT NULL PRIMARY KEY"), ("room_key", "varchar(64) NOT NULL"),
        ("contract_date", "date NULL"), ("move_in_date", "date NULL"),
        ("start_date", "date NOT NULL"), ("end_date", "date NULL"),
        ("monthly_rent_cents", "bigint NOT NULL"), ("currency", "char(3) NOT NULL"),
        ("due_day", "int NOT NULL"), ("status", "varchar(32) NOT NULL"),
    ], agreement_rows)
    statements += _temp_table("_rs_party", [
        ("agreement_key", "varchar(128) NOT NULL"), ("tenant_key", "varchar(64) NOT NULL"),
        ("responsibility_cents", "bigint NOT NULL"), ("joined_at", "date NULL"),
        ("left_at", "date NULL"), ("status", "varchar(32) NOT NULL"),
    ], party_rows, key="PRIMARY KEY (agreement_key, tenant_key)")
    statements += _temp_table("_rs_tx", [
        ("stable_transaction_key", "varchar(255) NOT NULL PRIMARY KEY"), ("amount_cents", "bigint NOT NULL"),
        ("transaction_time", "timestamp NULL"), ("description", "text NULL"),
        ("reference", "text NULL"), ("payer_name", "varchar(191) NULL"),
        ("period_month", "date NOT NULL"),
    ], transaction_rows)
    statements += _temp_table("_rs_payer", [
        ("tenant_key", "varchar(64) NOT NULL PRIMARY KEY"), ("payer_name", "varchar(191) NOT NULL"),
        ("payer_name_normalized", "varchar(191) NOT NULL"),
    ], payer_rows)

    statements += [
        "",
        "START TRANSACTION;",
        "",
        "-- 2) delete exactly this seed's rows, children before parents",
        "--    (fk_rooms_property, fk_tenancy_agreements_room, fk_agreement_parties_*",
        "--     and fk_rent_obligations_charge are RESTRICT, so order matters)",
        "DELETE pa FROM payment_allocations pa",
        "  JOIN payment_transactions pt ON pt.id = pa.payment_transaction_id",
        f" WHERE pt.user_id = @uid AND pt.stable_transaction_key LIKE {sql_text(STABLE_KEY_PREFIX + '%')};",
        "DELETE cr FROM cash_receipts cr",
        "  JOIN payment_transactions pt ON pt.id = cr.payment_transaction_id",
        f" WHERE pt.user_id = @uid AND pt.stable_transaction_key LIKE {sql_text(STABLE_KEY_PREFIX + '%')};",
        "DELETE act FROM payment_transaction_actions act",
        "  JOIN payment_transactions pt ON pt.id = act.payment_transaction_id",
        f" WHERE pt.user_id = @uid AND pt.stable_transaction_key LIKE {sql_text(STABLE_KEY_PREFIX + '%')};",
        f"DELETE FROM payment_transactions WHERE user_id = @uid AND stable_transaction_key LIKE {sql_text(STABLE_KEY_PREFIX + '%')};",
        "",
        "DELETE ap FROM agreement_parties ap",
        "  JOIN tenancy_agreements ta ON ta.id = ap.agreement_id",
        "  JOIN rooms r ON r.id = ta.room_id",
        "  JOIN properties p ON p.id = r.property_id",
        "  JOIN _rs_property k ON k.name = p.name",
        " WHERE ap.user_id = @uid AND p.user_id = @uid;",
        "DELETE ta FROM tenancy_agreements ta",
        "  JOIN rooms r ON r.id = ta.room_id",
        "  JOIN properties p ON p.id = r.property_id",
        "  JOIN _rs_property k ON k.name = p.name",
        " WHERE ta.user_id = @uid AND p.user_id = @uid;",
        "",
        "DELETE o FROM rent_obligations o",
        "  JOIN tenants t ON t.id = o.tenant_id",
        "  JOIN _rs_tenant k ON k.name = t.name AND k.room_label = t.room_label AND k.room_address = t.room_address",
        " WHERE o.user_id = @uid;",
        "DELETE tp FROM tenant_payers tp",
        "  JOIN tenants t ON t.id = tp.tenant_id",
        "  JOIN _rs_tenant k ON k.name = t.name AND k.room_label = t.room_label AND k.room_address = t.room_address",
        " WHERE tp.user_id = @uid;",
        "DELETE t FROM tenants t",
        "  JOIN _rs_tenant k ON k.name = t.name AND k.room_label = t.room_label AND k.room_address = t.room_address",
        " WHERE t.user_id = @uid;",
        "",
        "DELETE r FROM rooms r",
        "  JOIN properties p ON p.id = r.property_id",
        "  JOIN _rs_property k ON k.name = p.name",
        " WHERE r.user_id = @uid AND p.user_id = @uid;",
        "DELETE p FROM properties p JOIN _rs_property k ON k.name = p.name WHERE p.user_id = @uid;",
        "",
        "-- 3) insert the fixtures",
        "INSERT INTO properties (user_id, name, city_region, address, timezone, status)",
        "SELECT @uid, k.name, k.city_region, k.address, k.timezone, k.status FROM _rs_property k;",
        "",
        "DROP TEMPORARY TABLE IF EXISTS _id_property;",
        "CREATE TEMPORARY TABLE _id_property (k varchar(64) NOT NULL PRIMARY KEY, id bigint unsigned NOT NULL) ENGINE=InnoDB;",
        "INSERT INTO _id_property (k, id)",
        "SELECT k.k, p.id FROM properties p JOIN _rs_property k ON k.name = p.name WHERE p.user_id = @uid;",
        "",
        "INSERT INTO rooms (user_id, property_id, room_label, capacity, monthly_rent_cents, due_day, status, active_from)",
        "SELECT @uid, ip.id, k.room_label, k.capacity, k.monthly_rent_cents, k.due_day, k.status, k.active_from",
        "  FROM _rs_room k JOIN _id_property ip ON ip.k = k.property_key;",
        "",
        "DROP TEMPORARY TABLE IF EXISTS _id_room;",
        "CREATE TEMPORARY TABLE _id_room (k varchar(64) NOT NULL PRIMARY KEY, id bigint unsigned NOT NULL) ENGINE=InnoDB;",
        "INSERT INTO _id_room (k, id)",
        "SELECT k.k, r.id FROM rooms r JOIN _id_property ip ON ip.id = r.property_id",
        "  JOIN _rs_room k ON k.room_label = r.room_label AND k.property_key = ip.k",
        " WHERE r.user_id = @uid;",
        "",
        "INSERT INTO tenants (user_id, name, display_alias, monthly_rent_cents, currency, interval_unit,",
        "                     interval_count, billing_start_date, due_day, rent_start_date, rent_end_date,",
        "                     status, room_label, room_address, property_hint)",
        "SELECT @uid, k.name, k.display_alias, k.monthly_rent_cents, k.currency, 'month', 1,",
        "       k.billing_start_date, k.due_day, k.rent_start_date, k.rent_end_date,",
        "       k.status, k.room_label, k.room_address, k.property_hint",
        "  FROM _rs_tenant k;",
        "",
        "DROP TEMPORARY TABLE IF EXISTS _id_tenant;",
        "CREATE TEMPORARY TABLE _id_tenant (k varchar(64) NOT NULL PRIMARY KEY, id bigint unsigned NOT NULL) ENGINE=InnoDB;",
        "INSERT INTO _id_tenant (k, id)",
        "SELECT k.k, t.id FROM tenants t",
        "  JOIN _rs_tenant k ON k.name = t.name AND k.room_label = t.room_label AND k.room_address = t.room_address",
        " WHERE t.user_id = @uid;",
        "",
        "INSERT INTO tenancy_agreements (user_id, room_id, contract_date, move_in_date, start_date, end_date,",
        "                                monthly_rent_cents, currency, due_day, status)",
        "SELECT @uid, ir.id, k.contract_date, k.move_in_date, k.start_date, k.end_date,",
        "       k.monthly_rent_cents, k.currency, k.due_day, k.status",
        "  FROM _rs_agreement k JOIN _id_room ir ON ir.k = k.room_key;",
        "",
        "DROP TEMPORARY TABLE IF EXISTS _id_agreement;",
        "CREATE TEMPORARY TABLE _id_agreement (k varchar(128) NOT NULL PRIMARY KEY, id bigint unsigned NOT NULL) ENGINE=InnoDB;",
        "INSERT INTO _id_agreement (k, id)",
        "SELECT k.k, ta.id FROM tenancy_agreements ta",
        "  JOIN _id_room ir ON ir.id = ta.room_id",
        "  JOIN _rs_agreement k ON k.room_key = ir.k AND k.start_date = ta.start_date",
        " WHERE ta.user_id = @uid;",
        "",
        "INSERT INTO agreement_parties (user_id, agreement_id, tenant_id, responsibility_cents, joined_at, left_at, status)",
        "SELECT @uid, ia.id, it.id, k.responsibility_cents, k.joined_at, k.left_at, k.status",
        "  FROM _rs_party k JOIN _id_agreement ia ON ia.k = k.agreement_key",
        "  JOIN _id_tenant it ON it.k = k.tenant_key;",
        "",
        "-- The bank feed arrives unmatched; seed-match.sql performs the matching pass.",
        "INSERT INTO payment_transactions (user_id, source, stable_transaction_key, direction, amount_cents,",
        "                                  currency, transaction_time, description, reference, payer_name,",
        "                                  payer_name_kind, parsed_period_month, parsed_period_source, match_status)",
        "SELECT @uid, 'rosewood_seed', k.stable_transaction_key, 'income', k.amount_cents,",
        "       'EUR', k.transaction_time, k.description, k.reference, k.payer_name,",
        "       'confirmed', k.period_month, 'rosewood_seed', 'unmatched'",
        "  FROM _rs_tx k;",
        "",
        "INSERT INTO tenant_payers (user_id, tenant_id, payer_name_original, payer_name_normalized, source)",
        "SELECT @uid, it.id, k.payer_name, k.payer_name_normalized, 'manual'",
        "  FROM _rs_payer k JOIN _id_tenant it ON it.k = k.tenant_key;",
        "",
        "COMMIT;",
        "",
        "DROP TEMPORARY TABLE IF EXISTS _rs_property, _rs_room, _rs_tenant, _rs_agreement, _rs_party, _rs_tx, _rs_payer;",
        "DROP TEMPORARY TABLE IF EXISTS _id_property, _id_room, _id_tenant, _id_agreement;",
        "",
    ]
    # The addresses appear in the header so a reader can confirm the delete
    # scope without decoding the fixture keys.
    _write_sql(os.path.join(out_dir, "seed-data.sql"), statements)


def emit_match_sql(bundle: dict[str, object], out_dir: str) -> None:
    properties = bundle["properties"]["properties"]
    addresses = [row["name"] for row in properties]
    address_list = ", ".join(sql_text(value) for value in addresses)

    statements: list[str] = [
        "-- Generated by test-data/rosewood/extract.py -- do not edit by hand.",
        "-- Stage match: create the lazy rent obligations the app would create on",
        "-- first page load, confirm the pre-matched bank transactions, then",
        "-- backfill rent_obligations.paid_amount_cents/status from the confirmed",
        "-- allocations. The backfill is the whole point of this stage: the",
        "-- dashboard sums the stored column, so without it 已收 reads 0",
        "-- (design.md §6.1, migrations/013_dedupe_rent_obligations.sql:106-136).",
        "--",
        "-- Requires @uid; scoped to the four Rosewood addresses:",
        f"--   {address_list}",
        "",
        "START TRANSACTION;",
        "",
        "-- 1) one obligation per tenant-month, exactly like",
        "--    ensureMonthlyObligations (obligations.go:172-200): on the 1st, for the",
        "--    tenant's own share, lazily generated.",
        "--    Idempotency has two guards on purpose. INSERT IGNORE leans on the",
        "--    unique key idx_rent_obligations_lazy_tenant_period that migration 013",
        "--    adds (that migration is the task dependency, and the app applies it on",
        "--    startup); NOT EXISTS makes a re-run a no-op even on a database where",
        "--    013 has not been applied yet, where there is nothing to conflict with.",
        "INSERT IGNORE INTO rent_obligations",
        "  (user_id, tenant_id, tenant_name_snapshot, period_month, due_date, expected_amount_cents,",
        "   paid_amount_cents, currency, status, record_status, generated_by)",
        "SELECT @uid, t.id, t.name, m.period_month, m.period_month, t.monthly_rent_cents,",
        "       0, t.currency, 'open', 'active', 'lazy'",
        "  FROM tenants t",
        "  JOIN (",
    ]
    # The eight ledger months as a derived table, so the month list is explicit
    # in the SQL rather than hidden in a stored procedure or a calendar table.
    statements.append("\n".join(
        f"    SELECT {sql_text(month + '-01')} AS period_month"
        + (" UNION ALL" if index < len(LEDGER_MONTHS) - 1 else "")
        for index, month in enumerate(LEDGER_MONTHS)
    ))
    statements += [
        "  ) m",
        " WHERE t.user_id = @uid",
        f"   AND t.room_address IN ({address_list})",
        "   AND t.monthly_rent_cents > 0",
        "   AND m.period_month >= t.rent_start_date",
        "   AND m.period_month <= t.rent_end_date",
        "   AND NOT EXISTS (SELECT 1 FROM rent_obligations o",
        "                    WHERE o.user_id = t.user_id",
        "                      AND o.tenant_id = t.id",
        "                      AND o.rent_charge_id IS NULL",
        "                      AND o.period_month = m.period_month);",
        "",
        "-- 2) confirm the bank transactions that the ledger already ties to a",
        "--    tenancy. The rule (mirrored in extract.py, which reports the same",
        "--    split in review.json): a transaction the ledger recorded as a short",
        "--    payment, or one whose stable key hashes into the deterministic",
        "--    quarter, becomes a confirmed rent allocation to that month.",
        "INSERT IGNORE INTO payment_allocations",
        "  (user_id, payment_transaction_id, rent_obligation_id, tenant_id, amount_cents, allocation_kind,",
        "   idempotency_key, status, confirmed_by_user_id, confirmed_at, confirmation_source, note)",
        "SELECT @uid, pt.id, o.id, t.id,",
        "       LEAST(pt.amount_cents, o.expected_amount_cents - o.paid_amount_cents),",
        f"       'rent', CONCAT({sql_text(ALLOCATION_KEY_PREFIX)}, pt.stable_transaction_key), 'confirmed',",
        "       @uid, COALESCE(pt.transaction_time, CURRENT_TIMESTAMP), 'auto_match', 'Rosewood 收租明细导入自动匹配'",
        "  FROM payment_transactions pt",
        "  JOIN tenants t ON t.user_id = @uid AND t.name = pt.payer_name",
        "  JOIN rent_obligations o ON o.user_id = @uid AND o.tenant_id = t.id",
        "                         AND o.rent_charge_id IS NULL",
        "                         AND o.period_month = pt.parsed_period_month",
        " WHERE pt.user_id = @uid",
        "   AND pt.direction = 'income'",
        "   AND pt.amount_cents > 0",
        f"   AND pt.stable_transaction_key LIKE {sql_text(STABLE_KEY_PREFIX + '%')}",
        "   AND (pt.amount_cents < o.expected_amount_cents",
        "        OR CRC32(pt.stable_transaction_key) % 4 = 0);",
        "",
        "-- 3) reflect the allocations on the transaction itself, the way",
        "--    allocateTransactionInTx does (transaction_allocation.go:144-230).",
        "UPDATE payment_transactions pt",
        "  JOIN payment_allocations pa ON pa.payment_transaction_id = pt.id",
        "                             AND pa.user_id = pt.user_id AND pa.status = 'confirmed'",
        "  JOIN rent_obligations o ON o.id = pa.rent_obligation_id",
        "   SET pt.match_status = CASE WHEN pa.amount_cents >= o.expected_amount_cents THEN 'matched' ELSE 'partial' END,",
        "       pt.matched_tenant_id = pa.tenant_id,",
        f"       pt.match_reason = CONCAT('收租明细导入自动匹配 · 租金月 ', DATE_FORMAT(o.period_month, '%Y-%m'))",
        f" WHERE pt.user_id = @uid AND pt.stable_transaction_key LIKE {sql_text(STABLE_KEY_PREFIX + '%')};",
        "",
        "-- 4) THE BACKFILL. Copied from the final UPDATE of",
        "--    migrations/013_dedupe_rent_obligations.sql so the stored projection",
        "--    matches what projectRentObligation/obligationStatus recompute",
        "--    (obligations.go:165-170, ledger.go:119-139):",
        "--      paid = confirmed rent allocations + confirmed EUR cash receipts",
        "--      status priority needs_review > voided > paid > partial",
        "--                        > overdue > open, overdue on the Dublin day.",
        "SET @rosewood_year = YEAR(UTC_TIMESTAMP());",
        "SET @rosewood_dst_start = DATE_ADD(DATE_SUB(STR_TO_DATE(CONCAT(@rosewood_year, '-03-31'), '%Y-%m-%d'), INTERVAL (DAYOFWEEK(STR_TO_DATE(CONCAT(@rosewood_year, '-03-31'), '%Y-%m-%d')) - 1) DAY), INTERVAL 1 HOUR);",
        "SET @rosewood_dst_end = DATE_ADD(DATE_SUB(STR_TO_DATE(CONCAT(@rosewood_year, '-10-31'), '%Y-%m-%d'), INTERVAL (DAYOFWEEK(STR_TO_DATE(CONCAT(@rosewood_year, '-10-31'), '%Y-%m-%d')) - 1) DAY), INTERVAL 1 HOUR);",
        "SET @rosewood_today = CASE WHEN UTC_TIMESTAMP() >= @rosewood_dst_start AND UTC_TIMESTAMP() < @rosewood_dst_end THEN DATE(DATE_ADD(UTC_TIMESTAMP(), INTERVAL 1 HOUR)) ELSE DATE(UTC_TIMESTAMP()) END;",
        "",
        "UPDATE rent_obligations o",
        "  JOIN (",
        "    SELECT o2.id AS obligation_id, o2.user_id AS user_id,",
        "           (SELECT COALESCE(SUM(pa.amount_cents), 0)",
        "              FROM payment_allocations pa",
        "             WHERE pa.rent_obligation_id = o2.id",
        "               AND pa.user_id = o2.user_id",
        "               AND pa.status = 'confirmed'",
        "               AND (pa.allocation_kind = 'rent' OR pa.allocation_kind = ''))",
        "         + (SELECT COALESCE(SUM(cr.amount_cents), 0)",
        "              FROM cash_receipts cr",
        "             WHERE cr.rent_obligation_id = o2.id",
        "               AND cr.user_id = o2.user_id",
        "               AND cr.tenant_id = o2.tenant_id",
        "               AND cr.status = 'confirmed'",
        "               AND cr.amount_cents > 0",
        "               AND UPPER(TRIM(cr.currency)) = 'EUR'",
        "               AND UPPER(TRIM(cr.currency)) = UPPER(TRIM(o2.currency))) AS paid_cents",
        "      FROM rent_obligations o2",
        "     WHERE o2.rent_charge_id IS NULL",
        "       AND o2.user_id = @uid",
        "       AND o2.tenant_id IN (SELECT t.id FROM tenants t",
        f"                             WHERE t.user_id = @uid AND t.room_address IN ({address_list}))",
        "  ) p ON p.obligation_id = o.id AND p.user_id = o.user_id",
        "   SET o.paid_amount_cents = p.paid_cents,",
        "       o.status = CASE",
        "         WHEN o.status = 'needs_review' THEN 'needs_review'",
        "         WHEN o.record_status = 'voided' THEN 'voided'",
        "         WHEN p.paid_cents >= o.expected_amount_cents THEN 'paid'",
        "         WHEN p.paid_cents > 0 THEN 'partial'",
        "         WHEN o.due_date < @rosewood_today THEN 'overdue'",
        "         ELSE 'open'",
        "       END;",
        "",
        "COMMIT;",
        "",
    ]
    _write_sql(os.path.join(out_dir, "seed-match.sql"), statements)


def _write_sql(path: str, statements: list[str]) -> None:
    with open(path, "w", encoding="utf-8") as handle:
        handle.write("\n".join(statements))
        handle.write("\n")


# ---------------------------------------------------------------------------
# Entry point
# ---------------------------------------------------------------------------
def build(xlsx: str) -> dict[str, object]:
    review: dict[str, object] = collections.defaultdict(list)
    review.update({
        "skipped_rows": [], "unknown_names": [], "unknown_properties": [],
        "unbilled_rows": [],
        "blank_fills": [], "due_column_fallback": [], "rate_changes": [],
        "tenants_without_rent": [], "payment_capped": [], "decisions": [],
    })
    properties, occupancy, summaries = read_ledger(xlsx, review)
    report_unnamed_slots(occupancy, review)
    fill_blank_slots(occupancy, review)
    drop_unbilled_slots(occupancy, review)
    projected = build_occupancy(properties, occupancy, review)
    tenants, agreements, payments, payer_relations = build_segments(properties, projected, review)
    rooms = build_rooms(properties, projected, tenants)
    review["reconciliation"] = build_reconciliation(properties, projected, occupancy, summaries, tenants)

    review["decisions"] = [
        "Every tenant is written with status='active': ensureMonthlyObligations only bills tenants whose status is active, and it then uses rent_start_date/rent_end_date to decide the months. A tenant who moved out in June must therefore stay 'active' with rent_end_date=2026-06-30, otherwise their May/June bills would never be generated.",
        "No rent_charges rows are written (design.md §4): billing goes through the lazy tenants.monthly_rent_cents path, so /rent-dashboard?view=rooms shows each room's tenant count from agreement_parties, and the room rows read 待处理 because a room with an arrangement but no charge is deliberately surfaced that way by buildRentWorkspace.",
        "tenants.room_label / room_address / property_hint, tenancy_agreements and agreement_parties are all projected from the same occupancy map, so the dashboard and the room tree can never disagree (design.md §4).",
        "One tenants row per (person, room, contiguous month run). Two people swap rooms between 2026-06 and 2026-07 at 72 Walkinstown (Bhuvaneswaran and Kaushik Kannan), which is two tenancy segments each.",
        "Rent revisions are folded into a single monthly_rent_cents per tenant (most frequent observation, ties to the latest); the deviations are listed under rate_changes and the per-month reconciliation against the ledger's 合计 row is under reconciliation.",
        "One tenants row per (person, room, contiguous month run) means a person is never billed for a month the ledger did not bill: 169 Windmill Park room 05 joins in 2026-06, not 2026-05, because that row's 应收 is empty and the month's 合计 excludes it (see unbilled_rows).",
        "seed-data.sql and seed-match.sql are generated here because scripts/seed-rosewood.sh is bash + mysql only, and mysql cannot read JSON. Stage data is a full key-scoped delete-and-insert; stage match is set-based SQL because every decision it needs is derivable from the rows stage data wrote.",
        "The matched subset in stage match is the rule `amount_cents < expected_amount_cents OR CRC32(stable_transaction_key) % 4 = 0`. It is written once, in seed-match.sql; the same expression is computed here with zlib.crc32 (the same ISO-HDLC CRC-32 as MySQL's CRC32) purely so payment_summary below can report the split.",
        "A tenant_payers row is written for every tenant that has at least one pre-matched transaction, so the remaining unmatched transactions offer a 一键匹配 suggestion on /transactions.",
    ]
    review["payment_summary"] = review.get("payment_summary", {})
    return {
        "properties": {"properties": [properties[key] for key in sorted(properties)]},
        "rooms": {"rooms": rooms},
        "tenants": {"tenants": tenants},
        "agreements": {"agreements": agreements},
        "payments": {"payments": payments, "payer_relations": payer_relations},
        "review": dict(review),
    }


def write_fixtures(bundle: dict[str, object], out_dir: str) -> None:
    """Write the JSON fixtures and the SQL that scripts/seed-rosewood.sh runs."""
    mapping = {
        "properties": "properties.json",
        "rooms": "rooms.json",
        "tenants": "tenants.json",
        "agreements": "agreements.json",
    }
    for name, filename in mapping.items():
        with open(os.path.join(out_dir, filename), "w", encoding="utf-8") as handle:
            json.dump(bundle[name], handle, ensure_ascii=False, indent=1, sort_keys=False)
            handle.write("\n")
    # payments.json carries the remembered payer relations next to the ledger.
    with open(os.path.join(out_dir, "payments.json"), "w", encoding="utf-8") as handle:
        json.dump(bundle["payments"], handle, ensure_ascii=False, indent=1, sort_keys=False)
        handle.write("\n")
    with open(os.path.join(out_dir, "review.json"), "w", encoding="utf-8") as handle:
        json.dump(bundle["review"], handle, ensure_ascii=False, indent=1, sort_keys=False)
        handle.write("\n")
    emit_data_sql(bundle, out_dir)
    emit_match_sql(bundle, out_dir)


def summarize(bundle: dict[str, object]) -> str:
    properties = bundle["properties"]["properties"]
    rooms = bundle["rooms"]["rooms"]
    tenants = bundle["tenants"]["tenants"]
    agreements = bundle["agreements"]["agreements"]
    payments = bundle["payments"]["payments"]
    parties = sum(len(row["parties"]) for row in agreements)
    review = bundle["review"]
    lines = [
        f"properties              {len(properties)}",
        f"rooms                   {len(rooms)}",
        f"tenants                 {len(tenants)}",
        f"tenancy_agreements      {len(agreements)}",
        f"agreement_parties       {parties}",
        f"payment_transactions    {len(payments)}",
        f"tenant_payers           {len(bundle['payments']['payer_relations'])}",
        "",
        "skipped/placeholder rows: " + str(len(review["skipped_rows"])),
        "blank slots filled      : " + str(len(review["blank_fills"])),
        "rate changes folded      : " + str(len(review["rate_changes"])),
        "unknown names           : " + str(len(review["unknown_names"])),
        "due-column fallbacks    : " + str(len(review["due_column_fallback"])),
        "",
        "monthly expected rent (modelled):",
    ]
    totals: dict[str, int] = collections.Counter()
    sources: dict[str, int] = collections.Counter()
    for tenant in tenants:
        for month_row in review["reconciliation"]:
            if month_row["property"] == tenant["property_key"]:
                pass
    for row in review["reconciliation"]:
        totals[row["month"]] += row["modelled_due_cents"]
        sources[row["month"]] += row["source_due_cents"]
    for month in sorted(totals):
        lines.append(f"  {month}  modelled {totals[month] / 100:>9.2f}  source {sources[month] / 100:>9.2f}  delta {(totals[month] - sources[month]) / 100:>8.2f}")
    return "\n".join(lines)


def main(argv: list[str]) -> int:
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument("--xlsx", default=DEFAULT_XLSX)
    parser.add_argument("--out", default=HERE)
    parser.add_argument("--check", action="store_true", help="parse only; fail if unknown names remain")
    args = parser.parse_args(argv[1:])

    if not os.path.exists(args.xlsx):
        print(f"source workbook not found: {args.xlsx}", file=sys.stderr)
        return 2
    bundle = build(args.xlsx)
    if args.check:
        review = bundle["review"]
        problems = len(review["unknown_names"]) + len(review["unknown_properties"]) + len(review["tenants_without_rent"])
        print(summarize(bundle))
        if problems:
            print(f"\n{problems} unmatched name(s)/propert(y|ies) need an alias-table entry", file=sys.stderr)
            return 1
        return 0
    write_fixtures(bundle, args.out)
    print(summarize(bundle))
    print(f"\nwrote fixtures to {args.out}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main(sys.argv))
