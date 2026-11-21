import { useEffect } from "react";
import type { Domain } from "@/lib/types";
import { Select } from "./Form";

/** Selects one of the account's domains; picks the first one by default. */
export function DomainPicker({
  domains,
  value,
  onChange,
  filter,
}: {
  domains: Domain[] | undefined;
  value: string;
  onChange: (id: string) => void;
  filter?: (d: Domain) => boolean;
}) {
  const list = (domains ?? []).filter(filter ?? (() => true));
  useEffect(() => {
    if (!value && list[0]) onChange(list[0].id);
  }, [value, list, onChange]);
  return (
    <Select aria-label="Domain" className="w-64" value={value} onChange={(e) => onChange(e.target.value)}>
      {list.map((d) => (
        <option key={d.id} value={d.id}>
          {d.name}
        </option>
      ))}
    </Select>
  );
}
