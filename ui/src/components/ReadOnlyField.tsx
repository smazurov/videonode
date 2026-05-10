interface ReadOnlyFieldProps {
  label: string;
  value: string;
  mono?: boolean;
}

export function ReadOnlyField({ label, value, mono }: Readonly<ReadOnlyFieldProps>) {
  return (
    <div>
      <dt className="text-sm font-medium text-fg-subtle">{label}</dt>
      <dd className={`mt-1 text-sm text-fg ${mono ? 'font-mono' : ''}`}>
        {value}
      </dd>
    </div>
  );
}
