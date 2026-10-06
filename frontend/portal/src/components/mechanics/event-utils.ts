import type { EventType } from '@/services/api/types';

/**
 * Dot paths of an event type's declared properties (one level of nesting is
 * followed for object properties), for property-field suggestions.
 */
export function eventPropertyPaths(events: EventType[], slug: string): string[] {
  const schema = events.find((e) => e.slug === slug)?.property_schema;
  const props = schema?.properties ?? {};
  const paths: string[] = [];
  Object.entries(props).forEach(([name, def]) => {
    paths.push(name);
    const nested = (def as { properties?: Record<string, unknown> }).properties;
    if (def.type === 'object' && nested && typeof nested === 'object') {
      Object.keys(nested).forEach((child) => paths.push(`${name}.${child}`));
    }
  });
  return paths;
}
