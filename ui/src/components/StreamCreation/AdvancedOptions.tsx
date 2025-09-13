import { useEffect, useState } from 'react';
import { getFFmpegOptions, type FFmpegOption } from '../../lib/api';

interface AdvancedOptionsProps {
  selectedOptions: string[];
  onOptionsChange: (options: string[]) => void;
  disabled?: boolean;
  className?: string;
}

export function AdvancedOptions({ 
  selectedOptions, 
  onOptionsChange, 
  disabled = false,
  className = ''
}: Readonly<AdvancedOptionsProps>) {
  const [options, setOptions] = useState<FFmpegOption[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [expanded, setExpanded] = useState(false);

  useEffect(() => {
    loadOptions();
  }, []);

  const loadOptions = async () => {
    try {
      setLoading(true);
      const data = await getFFmpegOptions();
      setOptions(data.options);
      setError(null);
    } catch (error_) {
      setError('Failed to load advanced options');
      console.error('Failed to load FFmpeg options:', error_);
    } finally {
      setLoading(false);
    }
  };

  const toggleOption = (optionKey: string) => {
    if (selectedOptions.includes(optionKey)) {
      onOptionsChange(selectedOptions.filter(k => k !== optionKey));
    } else {
      onOptionsChange([...selectedOptions, optionKey]);
    }
  };

  const getConflictingOptions = (option: FFmpegOption): string[] => {
    const conflicts: string[] = [];
    
    // Check direct conflicts
    if (option.conflicts_with) {
      const directConflicts = option.conflicts_with.filter(conflict => 
        selectedOptions.includes(conflict)
      );
      conflicts.push(...directConflicts);
    }
    
    // Check exclusive group conflicts
    if (option.exclusive_group) {
      const groupConflicts = options
        .filter(o => 
          o.exclusive_group === option.exclusive_group && 
          o.key !== option.key &&
          selectedOptions.includes(o.key)
        )
        .map(o => o.key);
      conflicts.push(...groupConflicts);
    }
    
    return conflicts;
  };

  const isOptionDisabled = (option: FFmpegOption): boolean => {
    if (disabled) return true;
    if (selectedOptions.includes(option.key)) return false; // Always allow unchecking
    return getConflictingOptions(option).length > 0;
  };

  // Group options by category
  const optionsByCategory = options.reduce((acc, opt) => {
    const category = opt.category || 'Other';
    if (!acc[category]) acc[category] = [];
    acc[category].push(opt);
    return acc;
  }, {} as Record<string, FFmpegOption[]>);
  
  // Group options by exclusive group for radio button handling
  const exclusiveGroups = options.reduce((acc, opt) => {
    if (opt.exclusive_group) {
      if (!acc[opt.exclusive_group]) {
        acc[opt.exclusive_group] = [];
      }
      acc[opt.exclusive_group]!.push(opt);
    }
    return acc;
  }, {} as Record<string, FFmpegOption[]>);

  // Count non-default selected options
  const customOptionsCount = selectedOptions.filter(key => {
    const option = options.find(o => o.key === key);
    return option && !option.app_default;
  }).length;

  if (loading) {
    return (
      <div className={`text-sm text-gray-500 dark:text-gray-400 ${className}`}>
        Loading advanced options...
      </div>
    );
  }

  if (error) {
    return (
      <div className={`text-sm text-red-600 dark:text-red-400 ${className}`}>
        {error}
      </div>
    );
  }

  return (
    <details 
      className={`border border-gray-200 dark:border-gray-700 rounded-lg ${className}`}
      open={expanded}
      onToggle={(e) => setExpanded((e.target as HTMLDetailsElement).open)}
    >
      <summary className="px-4 py-3 cursor-pointer text-sm font-medium text-gray-700 dark:text-gray-300 hover:text-gray-900 dark:hover:text-gray-100 select-none">
        Advanced Options
        {customOptionsCount > 0 && (
          <span className="ml-2 text-xs text-gray-500 dark:text-gray-400">
            ({customOptionsCount} custom selected)
          </span>
        )}
      </summary>
      
      <div className="px-4 pb-4">
        <p className="text-xs text-gray-600 dark:text-gray-400 mb-4">
          Fine-tune FFmpeg behavior for your stream. Default options are pre-selected.
        </p>
        
        {Object.entries(optionsByCategory).map(([category, categoryOptions]) => {
          // Track which exclusive groups we've already rendered
          const renderedGroups = new Set<string>();
          
          return (
            <div key={category} className="mb-4">
              <h4 className="text-xs font-semibold text-gray-600 dark:text-gray-400 uppercase tracking-wider mb-2">
                {category}
              </h4>
              <div className="space-y-2">
                {categoryOptions.map((option) => {
                  const isSelected = selectedOptions.includes(option.key);
                  const isDisabled = isOptionDisabled(option);
                  const conflicts = getConflictingOptions(option);
                  
                  // Handle exclusive groups (radio buttons)
                  if (option.exclusive_group) {
                    // Skip if we've already rendered this group
                    if (renderedGroups.has(option.exclusive_group)) {
                      return null;
                    }
                    renderedGroups.add(option.exclusive_group);
                    
                    const groupOptions = exclusiveGroups[option.exclusive_group] || [];
                    
                    return (
                      <div key={`group-${option.exclusive_group}`} className="space-y-2">
                        {groupOptions.map((groupOption) => {
                          const isGroupOptionSelected = selectedOptions.includes(groupOption.key);
                          return (
                            <label
                              key={groupOption.key}
                              className={`
                                flex items-start space-x-3 p-2 rounded-md cursor-pointer
                                ${disabled ? 'opacity-50 cursor-not-allowed' : 'hover:bg-gray-50 dark:hover:bg-gray-800'}
                              `}
                            >
                              <input
                                type="radio"
                                name={`exclusive_${option.exclusive_group}`}
                                checked={isGroupOptionSelected}
                                onChange={() => {
                                  if (!disabled) {
                                    // Remove all options from this exclusive group, then add the selected one
                                    const newOptions = selectedOptions.filter(key => 
                                      !groupOptions.some(go => go.key === key)
                                    );
                                    newOptions.push(groupOption.key);
                                    onOptionsChange(newOptions);
                                  }
                                }}
                                disabled={disabled}
                                className="mt-0.5 h-4 w-4 text-blue-600 focus:ring-blue-500 border-gray-300 dark:border-gray-600 disabled:cursor-not-allowed"
                              />
                              <div className="flex-1 min-w-0">
                                <div className="flex items-center space-x-2">
                                  <span className="text-sm font-medium text-gray-700 dark:text-gray-300">
                                    {groupOption.name}
                                  </span>
                                  {groupOption.app_default && (
                                    <span className="inline-flex items-center px-2 py-0.5 rounded text-xs font-medium bg-blue-100 dark:bg-blue-900 text-blue-700 dark:text-blue-300">
                                      Default
                                    </span>
                                  )}
                                </div>
                                <p className="text-xs text-gray-500 dark:text-gray-400 mt-0.5">
                                  {groupOption.description}
                                </p>
                              </div>
                            </label>
                          );
                        })}
                      </div>
                    );
                  }
                  
                  // Regular checkbox for non-exclusive options
                  return (
                  <label
                    key={option.key}
                    className={`
                      flex items-start space-x-3 p-2 rounded-md cursor-pointer
                      ${isDisabled && !isSelected ? 'opacity-50 cursor-not-allowed' : ''}
                      ${!isDisabled ? 'hover:bg-gray-50 dark:hover:bg-gray-800' : ''}
                    `}
                  >
                    <input
                      type="checkbox"
                      checked={isSelected}
                      onChange={() => !isDisabled && toggleOption(option.key)}
                      disabled={isDisabled}
                      className="mt-0.5 h-4 w-4 text-blue-600 focus:ring-blue-500 border-gray-300 dark:border-gray-600 rounded disabled:cursor-not-allowed"
                    />
                    <div className="flex-1 min-w-0">
                      <div className="flex items-center space-x-2">
                        <span className="text-sm font-medium text-gray-700 dark:text-gray-300">
                          {option.name}
                        </span>
                        {option.app_default && (
                          <span className="inline-flex items-center px-2 py-0.5 rounded text-xs font-medium bg-blue-100 dark:bg-blue-900 text-blue-700 dark:text-blue-300">
                            Default
                          </span>
                        )}
                      </div>
                      <p className="text-xs text-gray-500 dark:text-gray-400 mt-0.5">
                        {option.description}
                      </p>
                      {conflicts.length > 0 && !isSelected && (
                        <p className="text-xs text-amber-600 dark:text-amber-400 mt-1">
                          Conflicts with: {conflicts.map(c => {
                            const conflictOption = options.find(o => o.key === c);
                            return conflictOption?.name || c;
                          }).join(', ')}
                        </p>
                      )}
                    </div>
                  </label>
                );
                })}
              </div>
            </div>
          );
        })}
        
        {Object.keys(optionsByCategory).length === 0 && (
          <p className="text-sm text-gray-500 dark:text-gray-400">
            No advanced options available
          </p>
        )}
      </div>
    </details>
  );
}