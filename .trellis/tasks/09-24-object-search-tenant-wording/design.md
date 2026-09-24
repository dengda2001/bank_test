# Design

`filterRoomPageRows` already searches its displayed `TenantNames`, but those can be aliases rather than legal names. Add searchable real-name/alias text from owner-scoped active room-plan members for the selected month; keep the displayed names unchanged. `filterPropertyPageRows` currently searches only property name, region, and address. Add owner-scoped room labels and active tenant real names/aliases for the same selected month, ideally with one batched lookup shared with room search. Keep filtering after the existing page row load and preserve month/status/collection/sort semantics. Avoid a per-property/per-room query added solely for search.

Both object list forms currently expose a `type=button` “搜索” control that only expands advanced filters on mobile. Add an actual `type=submit` search button and relabel the disclosure “筛选”; pressing Enter in the keyword field then uses native form submission. Update responsive CSS to keep both actions legible.

Audit rendered templates and user-facing server messages for “责任”; replace with terms that name the tenant and money: “当前责任人” → “入住租客”, “责任记录” → “租客租金记录”, “个人责任” → “个人月租” or “本月应付”, “责任月份” → “租金月份”, “责任归属” → “分配给哪位租客”. Keep backend symbols such as `rentObligation` and `ResponsibilityCents` intact. Review each context rather than globally replacing the word, then update copy assertions in tests.
