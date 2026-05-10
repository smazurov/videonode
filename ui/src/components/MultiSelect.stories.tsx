import type { Story, StoryDefault } from "@ladle/react";
import { useState } from "react";
import { MultiSelect } from "./MultiSelect";

export default {
  title: "Forms/MultiSelect",
} satisfies StoryDefault;

const OPTIONS = [
  { value: "a", label: "Alpha" },
  { value: "b", label: "Bravo" },
  { value: "c", label: "Charlie" },
  { value: "d", label: "Delta" },
];

export const Default: Story = () => {
  const [selected, setSelected] = useState<string[]>(["a"]);
  return (
    <MultiSelect
      options={OPTIONS}
      selected={selected}
      onChange={setSelected}
      placeholder="Pick some"
    />
  );
};

export const NoneSelected: Story = () => {
  const [selected, setSelected] = useState<string[]>([]);
  return (
    <MultiSelect
      options={OPTIONS}
      selected={selected}
      onChange={setSelected}
      placeholder="None"
    />
  );
};

export const AllSelected: Story = () => {
  const [selected, setSelected] = useState<string[]>(OPTIONS.map((o) => o.value));
  return (
    <MultiSelect
      options={OPTIONS}
      selected={selected}
      onChange={setSelected}
      placeholder="All"
    />
  );
};
