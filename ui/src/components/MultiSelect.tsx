import { Listbox, ListboxButton, ListboxOption, ListboxOptions } from '@headlessui/react';
import { ChevronDownIcon, CheckIcon } from '@heroicons/react/20/solid';

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
}

export function MultiSelect({
  options,
  selected,
  onChange,
  placeholder = 'Select...',
  className = '',
}: MultiSelectProps) {
  const allSelected = selected.length === options.length;
  const noneSelected = selected.length === 0;

  const getDisplayText = () => {
    if (noneSelected) return 'None';
    if (allSelected) return 'All';
    if (selected.length === 1) {
      const opt = options.find(o => o.value === selected[0]);
      return opt?.label ?? selected[0];
    }
    return `${selected.length} selected`;
  };

  return (
    <Listbox value={selected} onChange={onChange} multiple>
      <div className={`relative ${className}`}>
        <ListboxButton className="flex items-center gap-1 px-2 py-0.5 text-xs bg-gray-700 border border-gray-600 rounded text-gray-300 hover:bg-gray-600 cursor-pointer min-w-[70px]">
          <span className="truncate">{noneSelected ? placeholder : getDisplayText()}</span>
          <ChevronDownIcon className="w-3 h-3 shrink-0 text-gray-400" />
        </ListboxButton>

        <ListboxOptions
          anchor="bottom start"
          className="z-50 mt-1 w-max min-w-[120px] max-h-60 overflow-auto rounded bg-gray-800 border border-gray-600 shadow-lg focus:outline-none"
        >
          <div className="flex gap-1 px-2 py-1.5 border-b border-gray-700">
            <button
              type="button"
              onClick={e => {
                e.stopPropagation();
                onChange(options.map(o => o.value));
              }}
              className="text-xs text-blue-400 hover:text-blue-300"
            >
              All
            </button>
            <span className="text-gray-600">|</span>
            <button
              type="button"
              onClick={e => {
                e.stopPropagation();
                onChange([]);
              }}
              className="text-xs text-blue-400 hover:text-blue-300"
            >
              None
            </button>
          </div>

          {options.map(option => (
            <ListboxOption
              key={option.value}
              value={option.value}
              className="flex items-center gap-2 px-2 py-1 text-xs cursor-pointer select-none data-[focus]:bg-gray-700"
            >
              {({ selected: isSelected }) => (
                <>
                  <span
                    className={`flex items-center justify-center w-3.5 h-3.5 rounded border ${
                      isSelected
                        ? 'bg-blue-600 border-blue-600'
                        : 'border-gray-500 bg-transparent'
                    }`}
                  >
                    {isSelected && <CheckIcon className="w-2.5 h-2.5 text-white" />}
                  </span>
                  <span className={option.color ?? 'text-gray-300'}>
                    {option.label}
                  </span>
                </>
              )}
            </ListboxOption>
          ))}
        </ListboxOptions>
      </div>
    </Listbox>
  );
}
