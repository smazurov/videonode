import { Listbox, ListboxButton, ListboxOption, ListboxOptions } from '@headlessui/react';
import { CheckIcon, ChevronDownIcon } from '@heroicons/react/24/outline';

export interface MultiSelectOption {
  value: string;
  label: string;
  color?: string;
}

interface MultiSelectProps {
  readonly options: MultiSelectOption[];
  readonly selected: string[];
  readonly onChange: (selected: string[]) => void;
  readonly placeholder?: string;
  readonly className?: string;
  readonly label?: string;
}

export function MultiSelect({
  options,
  selected,
  onChange,
  placeholder = 'Select...',
  className = '',
  label,
}: MultiSelectProps) {
  const allSelected = selected.length === options.length;
  const noneSelected = selected.length === 0;

  const getDisplayText = () => {
    if (noneSelected) return 'None';
    if (allSelected) return 'All';
    if (selected.length === 1) {
      const opt = options.find((o) => o.value === selected[0]);
      return opt?.label ?? selected[0];
    }
    return `${selected.length} selected`;
  };

  return (
    <Listbox value={selected} onChange={onChange} multiple>
      <div className={`relative ${className}`}>
        <ListboxButton
          aria-label={label ?? placeholder}
          className="flex items-center gap-1 pl-2 pr-1.5 py-0.5 text-xs bg-surface-muted border border-border rounded text-fg cursor-pointer min-w-[70px] focus:outline-none focus-visible:ring-2 focus-visible:ring-focus-ring hover:bg-surface-muted/80"
        >
          <span className="flex-1 truncate">{noneSelected ? placeholder : getDisplayText()}</span>
          <ChevronDownIcon className="w-3.5 h-3.5 shrink-0 text-fg-subtle stroke-[1.5]" />
        </ListboxButton>

        <ListboxOptions
          anchor="bottom start"
          className="z-50 mt-1 w-max min-w-[120px] max-h-60 overflow-auto rounded bg-surface-raised border border-border shadow-lg focus:outline-none"
        >
          <div className="flex gap-1 px-2 py-1.5 border-b border-border">
            <button
              type="button"
              onClick={(e) => {
                e.stopPropagation();
                onChange(options.map((o) => o.value));
              }}
              className="text-xs text-accent hover:text-accent-hover focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-focus-ring rounded"
            >
              All
            </button>
            <span className="text-fg-subtle">|</span>
            <button
              type="button"
              onClick={(e) => {
                e.stopPropagation();
                onChange([]);
              }}
              className="text-xs text-accent hover:text-accent-hover focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-focus-ring rounded"
            >
              None
            </button>
          </div>

          {options.map((option) => (
            <ListboxOption
              key={option.value}
              value={option.value}
              className="flex items-center gap-2 px-2 py-1 text-xs cursor-pointer select-none data-[focus]:bg-surface-muted"
            >
              {({ selected: isSelected }) => (
                <>
                  <span
                    className={`flex items-center justify-center w-3.5 h-3.5 rounded border ${
                      isSelected
                        ? 'bg-accent border-accent'
                        : 'border-border-strong bg-transparent'
                    }`}
                  >
                    {isSelected && <CheckIcon className="w-2.5 h-2.5 text-accent-fg" />}
                  </span>
                  <span className={option.color ?? 'text-fg'}>{option.label}</span>
                </>
              )}
            </ListboxOption>
          ))}
        </ListboxOptions>
      </div>
    </Listbox>
  );
}
