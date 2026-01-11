import { useState, useEffect } from 'react';

interface MarketSearchProps {
  value: string;
  onChange: (value: string) => void;
}

export function MarketSearch({ value, onChange }: MarketSearchProps) {
  const [localValue, setLocalValue] = useState(value);

  useEffect(() => {
    const timer = setTimeout(() => {
      onChange(localValue);
    }, 300); // 300ms debounce
    
    return () => clearTimeout(timer);
  }, [localValue, onChange]);

  return (
    <div className="relative">
      <input
        type="text"
        placeholder="Search pair..."
        value={localValue}
        onChange={(e) => setLocalValue(e.target.value)}
        className="w-full md:w-64 px-4 py-2 pl-10 bg-gray-900 border border-gray-800 rounded-lg text-white placeholder-gray-500 focus:outline-none focus:ring-2 focus:ring-primary-500"
      />
      <svg
        className="absolute left-3 top-1/2 -translate-y-1/2 w-5 h-5 text-gray-500"
        fill="none"
        stroke="currentColor"
        viewBox="0 0 24 24"
      >
        <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z" />
      </svg>
    </div>
  );
}

