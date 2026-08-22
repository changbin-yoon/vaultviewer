import type { Role } from "../lib/api";

const LABEL: Record<Role, string> = { adm: "adm", dev: "dev", view: "view" };
const CLASS: Record<Role, string> = {
  adm: "tag tag-accent mono",
  dev: "tag tag-accent mono",
  view: "tag tag-neutral mono",
};

export function RoleTag({ role }: { role: Role }) {
  return <span className={CLASS[role]}>{LABEL[role]}</span>;
}
