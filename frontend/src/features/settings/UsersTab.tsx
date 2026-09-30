import { useState } from "react";
import { ErrorAlert } from "@/components/ErrorAlert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardBody, CardHeader } from "@/components/ui/card";
import { Field } from "@/components/ui/field";
import { Input, NativeSelect } from "@/components/ui/input";
import { Table, Td, Th } from "@/components/ui/table";
import { useToast } from "@/components/ui/toast-context";
import type { Schemas } from "@/lib/api";
import { useCreateUser, useUpdateUser, useUsers } from "@/services/users";

type Role = Schemas["UserRole"];

const ROLE_LABELS: Record<Role, string> = { ADMIN: "Administrador", MANAGER: "Gestor", OPERATOR: "Operacional", READ_ONLY: "Somente leitura" };
const ADMIN_ID = "00000000-0000-0000-0000-000000000001";

/** Responsáveis (usuários sem login no MVP) [PRD §34]. */
export function UsersTab() {
  const toast = useToast();
  const { data } = useUsers();
  const create = useCreateUser();
  const update = useUpdateUser();
  const [name, setName] = useState("");
  const [email, setEmail] = useState("");
  const [role, setRole] = useState<Role>("OPERATOR");

  return (
    <div className="grid gap-4">
      <Card>
        <CardHeader title="Responsáveis" description="Sem login no MVP: todas as alterações são registradas pelo usuário Administrador." />
        <CardBody className="p-0">
          <Table aria-label="Responsáveis">
            <thead>
              <tr>
                <Th>Nome</Th>
                <Th>E-mail</Th>
                <Th>Perfil</Th>
                <Th>Situação</Th>
                <Th className="w-32">
                  <span className="sr-only">Ação</span>
                </Th>
              </tr>
            </thead>
            <tbody>
              {data?.items.map((u) => (
                <tr key={u.id}>
                  <Td className="font-medium">{u.name}</Td>
                  <Td>{u.email ?? "—"}</Td>
                  <Td>{ROLE_LABELS[u.role]}</Td>
                  <Td>{u.active ? <Badge variant="success">Ativo</Badge> : <Badge variant="muted">Inativo</Badge>}</Td>
                  <Td>
                    {u.id !== ADMIN_ID ? (
                      <Button
                        variant="outline"
                        size="sm"
                        disabled={update.isPending}
                        onClick={() => update.mutate({ id: u.id, active: !u.active })}
                      >
                        {u.active ? "Inativar" : "Reativar"}
                      </Button>
                    ) : null}
                  </Td>
                </tr>
              ))}
            </tbody>
          </Table>
        </CardBody>
      </Card>
      <Card>
        <CardHeader title="Novo responsável" />
        <CardBody className="grid gap-3">
          <div className="flex flex-wrap items-end gap-3">
            <Field label="Nome" className="w-64">
              <Input value={name} onChange={(e) => setName(e.target.value)} />
            </Field>
            <Field label="E-mail" className="w-64">
              <Input type="email" value={email} onChange={(e) => setEmail(e.target.value)} />
            </Field>
            <Field label="Perfil" className="w-48">
              <NativeSelect value={role} onChange={(e) => setRole(e.target.value as Role)}>
                {(Object.keys(ROLE_LABELS) as Role[]).map((r) => (
                  <option key={r} value={r}>
                    {ROLE_LABELS[r]}
                  </option>
                ))}
              </NativeSelect>
            </Field>
            <Button
              disabled={!name.trim() || create.isPending}
              onClick={() =>
                create.mutate(
                  { name: name.trim(), email: email.trim() || null, role },
                  {
                    onSuccess: () => {
                      toast({ title: "Responsável cadastrado", variant: "success" });
                      setName("");
                      setEmail("");
                    },
                  },
                )
              }
            >
              Cadastrar
            </Button>
          </div>
          <ErrorAlert error={create.error ?? update.error} />
        </CardBody>
      </Card>
    </div>
  );
}
