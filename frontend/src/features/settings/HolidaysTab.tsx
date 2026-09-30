import { Trash2 } from "lucide-react";
import { useState } from "react";
import { ErrorAlert } from "@/components/ErrorAlert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardBody, CardHeader } from "@/components/ui/card";
import { Field } from "@/components/ui/field";
import { Input, NativeSelect } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
import { Table, Td, Th } from "@/components/ui/table";
import { useToast } from "@/components/ui/toast-context";
import { formatDate } from "@/lib/format";
import { UFS } from "@/lib/labels";
import { useCalendars, useCreateHoliday, useDeleteHoliday, useHolidays } from "@/services/calendar";

const SCOPE_LABELS = { NATIONAL: "Nacional", STATE: "Estadual", MUNICIPAL: "Municipal", CUSTOM: "Nacional (custom)" } as const;

/** Feriados: nacionais (seed) e estaduais/municipais configuráveis [PRD §21.2]. */
export function HolidaysTab({ today }: { today: Date }) {
  const toast = useToast();
  const [calendarId, setCalendarId] = useState("");
  const [year, setYear] = useState(today.getFullYear());
  const calendars = useCalendars();
  const holidays = useHolidays(calendarId, year);
  const create = useCreateHoliday();
  const remove = useDeleteHoliday();
  const [form, setForm] = useState({ scope: "STATE" as "NATIONAL" | "STATE" | "MUNICIPAL", state: "", city: "", date: "", name: "" });

  return (
    <div className="grid gap-4">
      <Card>
        <CardHeader
          title="Feriados"
          description="O calendário de cada empresa une os feriados nacionais, da UF e do município."
          actions={
            <div className="flex gap-2">
              <NativeSelect aria-label="Calendário" className="w-52" value={calendarId} onChange={(e) => setCalendarId(e.target.value)}>
                <option value="">Todos os calendários</option>
                {calendars.data?.items.map((c) => (
                  <option key={c.id} value={c.id}>
                    {c.name} ({c.holiday_count})
                  </option>
                ))}
              </NativeSelect>
              <NativeSelect aria-label="Ano" className="w-28" value={year} onChange={(e) => setYear(Number(e.target.value))}>
                {Array.from({ length: 7 }, (_, i) => 2025 + i).map((y) => (
                  <option key={y} value={y}>
                    {y}
                  </option>
                ))}
              </NativeSelect>
            </div>
          }
        />
        <CardBody className="p-0">
          {holidays.isPending ? (
            <Skeleton className="m-4 h-32" />
          ) : (
            <Table aria-label="Feriados">
              <thead>
                <tr>
                  <Th>Data</Th>
                  <Th>Feriado</Th>
                  <Th>Calendário</Th>
                  <Th>Abrangência</Th>
                  <Th className="w-12">
                    <span className="sr-only">Excluir</span>
                  </Th>
                </tr>
              </thead>
              <tbody>
                {holidays.data?.items.length === 0 ? (
                  <tr>
                    <Td colSpan={5} className="text-center text-muted-foreground">
                      Nenhum feriado neste filtro.
                    </Td>
                  </tr>
                ) : null}
                {holidays.data?.items.map((h) => (
                  <tr key={h.id}>
                    <Td className="tabular-nums">{formatDate(h.date)}</Td>
                    <Td>{h.name}</Td>
                    <Td>{h.calendar_name}</Td>
                    <Td>
                      <Badge variant={h.scope === "NATIONAL" ? "muted" : "default"}>{SCOPE_LABELS[h.scope]}</Badge>
                    </Td>
                    <Td>
                      {h.scope !== "NATIONAL" ? (
                        <Button
                          variant="ghost"
                          size="icon-sm"
                          aria-label={`Excluir ${h.name}`}
                          disabled={remove.isPending}
                          onClick={() => remove.mutate(h.id, { onSuccess: () => toast({ title: "Feriado excluído", variant: "success" }) })}
                        >
                          <Trash2 aria-hidden />
                        </Button>
                      ) : null}
                    </Td>
                  </tr>
                ))}
              </tbody>
            </Table>
          )}
        </CardBody>
      </Card>
      <Card>
        <CardHeader title="Novo feriado" description="Feriados estaduais e municipais passam a valer nos vencimentos das próximas gerações." />
        <CardBody className="grid gap-3">
          <div className="flex flex-wrap items-end gap-3">
            <Field label="Abrangência" className="w-40">
              <NativeSelect value={form.scope} onChange={(e) => setForm({ ...form, scope: e.target.value as typeof form.scope })}>
                <option value="STATE">Estadual</option>
                <option value="MUNICIPAL">Municipal</option>
                <option value="NATIONAL">Nacional</option>
              </NativeSelect>
            </Field>
            {form.scope !== "NATIONAL" ? (
              <Field label="UF" className="w-24">
                <NativeSelect value={form.state} onChange={(e) => setForm({ ...form, state: e.target.value })}>
                  <option value="">—</option>
                  {UFS.map((uf) => (
                    <option key={uf} value={uf}>
                      {uf}
                    </option>
                  ))}
                </NativeSelect>
              </Field>
            ) : null}
            {form.scope === "MUNICIPAL" ? (
              <Field label="Município" className="w-52">
                <Input value={form.city} onChange={(e) => setForm({ ...form, city: e.target.value })} />
              </Field>
            ) : null}
            <Field label="Data">
              <Input type="date" value={form.date} onChange={(e) => setForm({ ...form, date: e.target.value })} />
            </Field>
            <Field label="Nome" className="w-64">
              <Input value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} />
            </Field>
            <Button
              disabled={!form.date || !form.name.trim() || create.isPending}
              onClick={() =>
                create.mutate(
                  {
                    scope: form.scope,
                    date: form.date,
                    name: form.name.trim(),
                    state: form.scope === "NATIONAL" ? null : form.state || null,
                    city: form.scope === "MUNICIPAL" ? form.city.trim() || null : null,
                  },
                  {
                    onSuccess: () => {
                      toast({ title: "Feriado cadastrado", variant: "success" });
                      setForm({ ...form, date: "", name: "" });
                    },
                  },
                )
              }
            >
              Cadastrar feriado
            </Button>
          </div>
          <ErrorAlert error={create.error ?? remove.error} />
        </CardBody>
      </Card>
    </div>
  );
}
