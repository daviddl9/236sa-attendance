# Phase 0 Research: Ops-Group Seed Rules & Roster Matching

**Date**: 2026-08-17 · **Feature**: [spec.md](./spec.md)

## 1. Roster file structure

File: `236SA 2026 ICT 6 callup (test) 2 (1).xlsx` (password `yeoec`), sheet `236 SA`, 431 people.

| Index | Column | Notes |
|---|---|---|
| 1 | NRIC | Last 5, e.g. `1290E` |
| 2 | Rank | REC…LTC (SAF rank set) |
| 3 | Name | `LIM KIAK PHENG` |
| 4 | (admin no.) | ignore |
| 5 | Position Description | CO, FDO, MEDIC, MTO, QM, BSM, … |
| 6 | Unit | always `236 SA` |
| 7 | Sub-Unit 1 | BN HQ · BN OPS CEN · BN RECCE & SURVEY TM · FIRE DIRECTION CEN · FIELD ARTY BTY A/B · HQ BTY · S1 BR · NON-ESTAB · RSM GP · HL 236 SA |
| 8 | Sub-Unit 2 | BTY COMMAND POST · BTY HQ · BTY RECCE GP · COMBAT TRAIN · GUN DET 1–6 · HQ COMBAT TRAIN · MEDICAL PL · MT PL · PERSONNEL SP PL · QM & SVCS PL · S1 CELL · SIGNAL PL · No Data |

## 2. Sub-unit structure facts

- `S1 BR` (48) = `PERSONNEL SP PL` (35) + `S1 CELL` (13).
- `HQ BTY` (139) = BTY HQ (4) + HQ COMBAT TRAIN (6) + MEDICAL PL (16) + MT PL (39) + QM & SVCS PL (61) + SIGNAL PL (13).
- `FIELD ARTY BTY A/B` each carry their own BTY COMMAND POST, BTY HQ, BTY RECCE GP, COMBAT TRAIN, GUN DET 1–6 — the battery-level elements.
- `FIRE DIRECTION CEN` (11) and `BN RECCE & SURVEY TM` (8) are battalion-level, rows have Sub-Unit 2 = "No Data".

## 3. Confirmed seed rules (user-approved)

Decision: battery-level personnel are excluded from functional ops groups and accounted at battery level.

| Group | Rule | n |
|---|---|---|
| RnS | sub1 = `BN RECCE & SURVEY TM` | 8 |
| FDC/BOC | sub1 = `FIRE DIRECTION CEN` | 11 |
| BCS | sub2 = `MEDICAL PL` | 16 |
| CSS | (sub1=`S1 BR` & sub2=`S1 CELL`) + sub2 ∈ {MT PL, MEDICAL PL, HQ COMBAT TRAIN} + posdesc=`S4/OC HQ` + (sub2=`BTY HQ` & posdesc=`BSM`) | 76 |
| PSO | rank ≥ 1SG (rank-order map) | 40 |
| A Bty | sub1 = `FIELD ARTY BTY A` | 90 |
| B Bty | sub1 = `FIELD ARTY BTY B` | 89 |
| HQ Bty | sub1 = `HQ BTY` | 139 |
| MT Platoon | sub2 = `MT PL` | 39 |
| Technicians | vocation ∈ {`AUTO TECH`, `AUTO SPEC TECH`, `ARMT TECH`, `ARMT SPEC TECH`} | 16 |
| CSS Commanders | CSS members with rank ≥ 3SG | 22 |
| A Commanders | A Bty members with rank ≥ 3SG | 29 |
| B Commanders | B Bty members with rank ≥ 3SG | 32 |
| HQ Commanders | HQ Bty members with rank ≥ 3SG | 29 |
| All Commanders | all roster members with rank ≥ 3SG | 123 |

Rationale for exclusions:

- **FDC/BOC**: BTY COMMAND POST people are battery-level (each battery owns one) — excluded so they're accounted in A/B Bty.
- **RnS**: BTY RECCE GP is battery-level — excluded.
- **CSS**: PERSONNEL SP PL (S1's ops platoon) and QM & SVCS PL (user will hand-pick QM people later) and battery COMBAT TRAINs excluded. HQ-combat-train (battalion-level) retained.
- **BCS**: is a subset concept; medics sit in both BCS and CSS per user decision.

Alternatives considered: including battery elements in FDC/BOC and RnS (rejected: double-accounting, user wants battery-level grouping for accounting).

## 4. User matching strategy

- Key: `nric_last5` (uppercase, normalized via existing `normalizeNRICLast5`).
- Confirmation: `upper(full_name) = upper($1)`.
- Query verified users; if nric match finds 1 → use. If multiple → prefer exact name; if still ambiguous → skip + report.
- Unmatched rows: skip + report (count + per-group list).
- Cross-battery name collisions are handled by the nric-first strategy (nric is unique-ish enough).

## 5. Idempotence

- Groups: reuse by `created_by` + `lower(name)` (the existing unique index). `INSERT ... ON CONFLICT DO NOTHING` when creating.
- Members: `INSERT ... ON CONFLICT DO NOTHING`. Never DELETE during seed.
- Manual additions survive re-seeds by construction.
