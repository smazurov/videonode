import { Checkbox } from '../Checkbox';

interface SourceTestModeToggleProps {
  value: boolean;
  onChange: (next: boolean) => void;
  disabled?: boolean;
}

export function SourceTestModeToggle({
  value,
  onChange,
  disabled = false,
}: Readonly<SourceTestModeToggleProps>) {
  return (
    <Checkbox
      checked={value}
      onChange={(e) => onChange(e.target.checked)}
      disabled={disabled}
      label="Test mode"
      description="Use the RPC-driven test-pattern producer instead of a real V4L2 device. Lets the full pipeline run without any hardware attached."
    />
  );
}
