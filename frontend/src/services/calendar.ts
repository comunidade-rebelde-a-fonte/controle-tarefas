import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, type Schemas } from "@/lib/api";

export type Holiday = Schemas["Holiday"];
export type BusinessCalendar = Schemas["Calendar"];

export const calendarKeys = {
  calendars: ["calendar", "calendars"] as const,
  holidays: (calendarId: string, year: number) => ["calendar", "holidays", calendarId, year] as const,
};

export function useCalendars() {
  return useQuery({ queryKey: calendarKeys.calendars, queryFn: ({ signal }) => api.get("/calendars", { signal }) });
}

export function useHolidays(calendarId: string, year: number) {
  return useQuery({
    queryKey: calendarKeys.holidays(calendarId, year),
    queryFn: ({ signal }) => api.get("/holidays", { query: { year, ...(calendarId ? { calendar_id: calendarId } : {}) }, signal }),
  });
}

export function useCreateHoliday() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: Schemas["HolidayInput"]) => api.post("/holidays", { body }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["calendar"] }),
  });
}

export function useDeleteHoliday() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => api.delete("/holidays/{id}", { path: { id } }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["calendar"] }),
  });
}
