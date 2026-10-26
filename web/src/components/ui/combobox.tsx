import { useEffect, useId, useRef, useState } from "react";
import { Check, ChevronDown } from "lucide-react";

// Editable context filter: choosing an option is optional; arbitrary text is valid.
export function Combobox({
  id,
  value,
  onChange,
  options,
  placeholder,
}: {
  id: string;
  value: string;
  onChange: (value: string) => void;
  options: string[];
  placeholder?: string;
}) {
  const listId = useId();
  const input = useRef<HTMLInputElement>(null);
  const activeOption = useRef<HTMLDivElement>(null);
  const [open, setOpen] = useState(false);
  const [active, setActive] = useState<string | null>(null);
  const matching = options.filter((option) =>
    option.toLowerCase().includes(value.toLowerCase()),
  );
  const activeIndex = active === null ? -1 : matching.indexOf(active);
  useEffect(() => {
    if (open) activeOption.current?.scrollIntoView({ block: "nearest" });
  }, [open, active]);
  function choose(option: string) {
    onChange(option);
    setOpen(false);
    setActive(null);
  }
  return (
    <div
      className="combobox"
      onBlur={(event) => {
        if (!event.currentTarget.contains(event.relatedTarget)) {
          setOpen(false);
          setActive(null);
        }
      }}
    >
      <input
        id={id}
        ref={input}
        role="combobox"
        autoComplete="off"
        aria-autocomplete="list"
        aria-expanded={open}
        aria-controls={open ? listId : undefined}
        aria-activedescendant={
          open && activeIndex >= 0 ? `${listId}-${activeIndex}` : undefined
        }
        placeholder={placeholder}
        value={value}
        onFocus={() => setOpen(true)}
        onClick={() => setOpen(true)}
        onChange={(event) => {
          onChange(event.target.value);
          setOpen(true);
          setActive(null);
        }}
        onKeyDown={(event) => {
          if (event.key === "ArrowDown" || event.key === "ArrowUp") {
            event.preventDefault();
            const direction = event.key === "ArrowDown" ? 1 : -1;
            const next =
              !open || activeIndex < 0
                ? direction === 1
                  ? 0
                  : matching.length - 1
                : (activeIndex + direction + matching.length) % matching.length;
            setOpen(true);
            setActive(matching[next] ?? null);
          } else if (open && (event.key === "Home" || event.key === "End")) {
            event.preventDefault();
            setActive(
              matching[event.key === "Home" ? 0 : matching.length - 1] ?? null,
            );
          } else if (open && event.key === "Enter") {
            event.preventDefault();
            if (activeIndex >= 0) choose(matching[activeIndex]);
            else setOpen(false);
          } else if (open && event.key === "Escape") {
            event.preventDefault();
            event.stopPropagation();
            setOpen(false);
            setActive(null);
          } else if (event.key === "Tab") {
            setOpen(false);
            setActive(null);
          }
        }}
      />
      <button
        type="button"
        className="combobox-toggle"
        tabIndex={-1}
        aria-label="Toggle suggestions"
        aria-controls={open ? listId : undefined}
        aria-expanded={open}
        onMouseDown={(event) => event.preventDefault()}
        onClick={() => {
          input.current?.focus();
          setOpen(!open);
          setActive(null);
        }}
      >
        <ChevronDown size={16} aria-hidden="true" />
      </button>
      {open && (
        <div
          id={listId}
          role="listbox"
          aria-label="Context suggestions"
          className="combobox-options"
        >
          {matching.map((option, index) => (
            <div
              key={option}
              id={`${listId}-${index}`}
              ref={index === activeIndex ? activeOption : undefined}
              role="option"
              aria-selected={option === value}
              data-active={index === activeIndex}
              onMouseDown={(event) => event.preventDefault()}
              onMouseMove={() => setActive(option)}
              onClick={() => choose(option)}
            >
              <span>{option}</span>
              {option === value && <Check size={14} aria-hidden="true" />}
            </div>
          ))}
          {!matching.length && (
            <div className="combobox-empty">
              No matching contexts. You can keep this text as a filter.
            </div>
          )}
        </div>
      )}
    </div>
  );
}
