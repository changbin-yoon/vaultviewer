import type { Role } from "../lib/api";

const LABEL: Record<Role, string> = { adm: "adm", dev: "dev", view: "view" };

// view/dev/adm is a real trust ordering (read-only -> read-write ->
// destructive), so the tier count is meaningful, not decorative — see the
// .al-clearance rules in index.css.
const TIER: Record<Role, 1 | 2 | 3> = { view: 1, dev: 2, adm: 3 };

export function RoleTag({ role }: { role: Role }) {
  return (
    <span className="al-clearance" data-role={role} data-tier={TIER[role]}>
      <span className="al-clearance-ticks" aria-hidden="true">
        <span />
        <span />
        <span />
      </span>
      {LABEL[role]}
    </span>
  );
}
