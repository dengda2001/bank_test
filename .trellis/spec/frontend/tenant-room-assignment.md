# Tenant Room Assignment

> Executable contract for the optional occupancy and rent assignment block in
> the server-rendered tenant creation drawer.

## 1. Scope / Trigger

- Trigger: changing the new-tenant drawer, room-create drawer, their form
  handlers, or client-side room/rent preview behavior.
- Applies to `tenant-form-drawer.html`, `room-create-drawer.html`,
  `entity-drawers.css`, `handleTenants`, and the composite room/tenant services.
- This is creation-time setup only. Editing a tenant remains an identity and
  payer-profile form; moving an existing tenant belongs to the room plan flow.

## 2. Signatures

- `GET /tenants?add=1&property_id=<id>&room_id=<id>&arrangement_start_month=YYYY-MM`
  preselects the optional assignment block.
- Tenant creation posts `property_id`, `room_id`, `arrangement_start_month`,
  `plan_version`, `room_plan`, and optional `new_responsibility` in addition to
  the normal tenant identity fields.
- `room_plan` is a JSON array of existing members:
  `[{"tenant_id": 12, "responsibility_cents": 40000}]`.
- Room creation posts required `monthly_rent`, `effective_month`, and `due_day`;
  `save_and_setup` redirects to the preselected tenant-create URL.

## 3. Contracts

- The room selector is filtered by the selected property and by a plan that is
  active in the selected month. It is optional: no room means the request only
  creates a tenant profile.
- `PlansJSON` is display/preview data only. It may show the room's monthly rent,
  due day, and existing occupants, but the server re-reads the plan, its member
  IDs, the room property, and the timeline version before writing.
- The new tenant has a separate optional rent input. Existing occupants render
  in `.tenant-existing-occupants`: names and current responsibilities are text,
  and only an optional adjustment amount is editable.
- Blank amounts mean unspecified. Client preview and server validation use the
  same visible rule: fixed positive amounts are removed from the room total and
  the positive remainder is divided between blank occupants; all fixed amounts
  must sum exactly to the room total when no blank occupant remains.
- Build dynamic occupant rows with DOM APIs and `textContent`; do not use
  `innerHTML` with tenant names or values. Keep native controls as the source
  of submitted values.

## 4. Validation & Error Matrix

| Condition | UI / handler behavior |
|---|---|
| No property or room | Save normal tenant profile only |
| Room does not have a plan for the chosen month | Hide it from the selector; server also rejects a tampered request |
| Browser amount malformed or fixed amounts leave no positive remainder | Block submit with a form message; server returns `tenant_room_invalid` |
| Member IDs or timeline version changed | Return `tenant_room_stale` and reopen the drawer with its preselection |
| Tenant belongs to another room in the month | Return `tenant_room_conflict` |
| Rent facts are locked | Return `tenant_room_locked` |

## 5. Good / Base / Bad Cases

- Good: a new room is created with a vacant rent rule, then “保存并设置租客”
  opens the tenant drawer with its property, room, and start month selected.
- Base: a user adds a tenant without selecting a room; no plan data is posted or
  mutated.
- Bad: use the room option's JSON price, due day, or member list as an authority
  on the server, or place existing tenant identity fields in editable inputs.

## 6. Tests Required

- Rendered-template tests assert room rent fields/defaults, tenant-create-only
  plan fields, the grouping hint, and absence of those fields from tenant edit.
- Parser tests reject unknown JSON fields, malformed IDs/months/amounts, and
  accept an omitted optional room assignment.
- MySQL service tests prove a first occupant is atomically added to an empty
  plan and a stale version rolls the new tenant back.
- Browser verification, when the local browser bridge is available, checks the
  property-to-room filter, month switch, visible room rent, grey existing-member
  block, equal/partial split preview, and the no-room profile path at desktop
  and mobile widths.

## 7. Wrong vs Correct

Wrong:

```js
existingRows.innerHTML = occupants.map((occupant) => `<input value="${occupant.name}">`).join("");
```

Correct:

```js
const name = document.createElement("strong");
name.textContent = occupant.tenant_name;
row.append(name);
```

The correct version keeps existing identity read-only and does not turn tenant
data into markup.
