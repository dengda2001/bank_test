# Tenant Room Assignment

> Executable contract for room creation, existing-tenant availability, and the
> server-rendered tenant creation drawer.

## 1. Scope / Trigger

- Trigger: changing the new-tenant drawer, room-create drawer, room occupancy
  editor, their handlers, or client-side room/rent preview behavior.
- Applies to `tenant-form-drawer.html`, `room-create-drawer.html`,
  `room-detail.html`, `room-tenant-availability.js`, `entity-drawers.css`,
  `handleTenants`, and the composite room/tenant services.
- This is creation-time setup only. Editing a tenant remains an identity and
  payer-profile form; moving an existing tenant belongs to the room plan flow.

## 2. Signatures

- `GET /tenants?add=1&property_id=<id>&room_id=<id>&arrangement_start_month=YYYY-MM`
  preselects the optional assignment block.
- Tenant creation posts `property_id`, `room_id`, `arrangement_start_month`,
  `plan_version`, `room_plan`, and optional `new_responsibility` in addition to
  the normal tenant identity fields.
- A room-bound tenant drawer also receives `return_room_id=<same room ID>`.
  Its property and room controls are fixed, while hidden `property_id` and
  `room_id` fields carry the assignment. The successful POST returns to the
  originating room; validation errors reopen the tenant drawer.
- `room_plan` is a JSON array of existing members:
  `[{"tenant_id": 12, "responsibility_cents": 40000}]`.
- Room creation posts required `monthly_rent`, `effective_month`, and `due_day`.
  `action=save` creates a vacant plan. `action=save_and_setup` plus
  `tenant_choice=existing&tenant_id=<id>` creates the room and occupied plan
  atomically; `tenant_choice=new` creates a vacant plan then redirects to the
  preselected tenant-create URL. Omitted `tenant_choice` uses `new` for older
  forms.

## 3. Contracts

- The tenant-create room selector lists only rooms in the selected property.
  A room without a plan for the selected month remains visible, but the form
  marks it invalid and explains that its rent rule must be set first. With no
  room selected, the request only creates a tenant profile.
- Existing-tenant choices in room creation and the room plan editor are
  unavailable if another room has a tenant plan ending in or after the chosen
  effective month, including future plans. The selector marks these options
  disabled and names the conflicting room; a nearby link opens that room's
  occupancy editor so the user can remove the tenant first. Changing the month
  refreshes this presentation state. `SaveRoomRentPlan` checks it again under
  tenant locks, so the browser payload never authorizes a transfer.
- The room occupancy editor can save an empty member list. Removing its final
  member keeps the rent schedule while unbinding the tenant. If no rent plan
  exists yet, save that empty rule before opening the room-bound tenant drawer.
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
| Room does not have a plan for the chosen month | Keep the property-scoped room visible, show an invalid-selection message, and reject POST |
| Existing tenant occupies another room from the chosen month onward | Disable and flag the choice with an unbind link; a stale/forged POST returns `tenant_room_conflict` and rolls back room creation |
| Room-bound new-tenant form posts a different room ID | Return `tenant_room_invalid` before creating a tenant |
| Browser amount malformed or fixed amounts leave no positive remainder | Block submit with a form message; server returns `tenant_room_invalid` |
| Member IDs or timeline version changed | Return `tenant_room_stale` and reopen the drawer with its preselection |
| Tenant belongs to another room in the month | Return `tenant_room_conflict` |
| Rent facts are locked | Return `tenant_room_locked` |

## 5. Good / Base / Bad Cases

- Good: “保存并设置租客” with an available existing tenant creates the room,
  its first plan member, and current-month facts in one transaction.
- Good: “保存并设置租客” with a new tenant opens the tenant drawer with its
  property, room, and start month selected.
- Base: a user adds a tenant without selecting a room; no plan data is posted or
  mutated.
- Bad: enable a conflicting option after a month switch without rechecking it
  on POST, use the room option's JSON price/due day/member list as authority,
  or place existing tenant identity fields in editable inputs.

## 6. Tests Required

- Rendered-template tests assert room rent fields/defaults, tenant-create-only
  plan fields, the grouping hint, and absence of those fields from tenant edit.
- Parser tests reject unknown JSON fields, malformed IDs/months/amounts, and
  accept an omitted optional room assignment.
- MySQL service tests prove a first occupant is atomically added to an empty
  plan, an existing occupant can be added during room creation, and conflict
  or stale version rolls the new record back.
- Unit/template tests cover inclusive end-month and future-plan conflicts,
  disabled choices and unbind links, a fixed room-bound tenant form, and a
  room-only success URL that matches the submitted assignment.
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
