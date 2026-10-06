import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { useEventsQuery } from '@/services/queries/events';

/** Sentinel for "no event type" (Radix Select items cannot have an empty value). */
const ANY = '__any';

interface EventTypeSelectProps {
  /** Selected event type slug, or "". */
  value: string;
  onChange: (slug: string) => void;
  id?: string;
  /** When set, an extra first option that maps to "" (e.g. "Any event type"). */
  emptyLabel?: string;
  placeholder?: string;
  disabled?: boolean;
  className?: string;
}

/**
 * Picks an event type slug from the tenant's catalogue (GET /events, global +
 * tenant types). A saved slug that is no longer in the catalogue stays
 * selectable so editing never silently drops it.
 */
export function EventTypeSelect({
  value,
  onChange,
  id,
  emptyLabel,
  placeholder = 'Select an event type',
  disabled,
  className,
}: EventTypeSelectProps) {
  const { data: events = [], isLoading, error } = useEventsQuery();
  const known = events.some((e) => e.slug === value);

  return (
    <Select
      value={value ? value : emptyLabel ? ANY : undefined}
      onValueChange={(v) => onChange(v === ANY ? '' : v)}
      disabled={disabled}
    >
      <SelectTrigger id={id} className={className}>
        <SelectValue placeholder={isLoading ? 'Loading event types...' : placeholder} />
      </SelectTrigger>
      <SelectContent>
        {emptyLabel && <SelectItem value={ANY}>{emptyLabel}</SelectItem>}
        {value && !known && !isLoading && (
          <SelectItem value={value}>
            {value} <span className="ml-2 text-xs text-muted-foreground">(not in catalogue)</span>
          </SelectItem>
        )}
        {events.length === 0 && !emptyLabel && !value ? (
          <SelectItem value="__none" disabled>
            {error ? 'Failed to load event types' : isLoading ? 'Loading...' : 'No event types: create one under Events'}
          </SelectItem>
        ) : (
          events.map((event) => (
            <SelectItem key={event.id} value={event.slug} disabled={!event.is_active && event.slug !== value}>
              {event.name}
              <span className="ml-2 text-xs text-muted-foreground font-mono">{event.slug}</span>
              {!event.is_active && <span className="ml-1 text-xs text-muted-foreground">(inactive)</span>}
            </SelectItem>
          ))
        )}
      </SelectContent>
    </Select>
  );
}
