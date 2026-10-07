#!/bin/sh
# Inicializa o MinIO dos originais da Conciliação (serviço minio-init do docker-compose.yml).
#
# - bucket S3_BUCKET com versionamento ligado [spec 04 DEC-004, 10 §3];
# - usuário da aplicação (S3_ACCESS_KEY) sem permissão de apagar objeto, versão ou bucket, nem de
#   desligar o versionamento: o original do cliente é imutável [spec 09 §5]. Só o administrador
#   (S3_ROOT_USER) pode apagar, e nenhum fluxo da aplicação usa essa credencial.
#
# Idempotente: roda a cada `make dev`.
set -eu

: "${S3_ROOT_USER:?defina S3_ROOT_USER}"
: "${S3_ROOT_PASSWORD:?defina S3_ROOT_PASSWORD}"
: "${S3_ACCESS_KEY:?defina S3_ACCESS_KEY}"
: "${S3_SECRET_KEY:?defina S3_SECRET_KEY}"
: "${S3_BUCKET:?defina S3_BUCKET}"
POLICY=conciliacao-app

mc alias set local http://minio:9000 "$S3_ROOT_USER" "$S3_ROOT_PASSWORD" >/dev/null
mc mb --ignore-existing "local/$S3_BUCKET"
mc version enable "local/$S3_BUCKET"

policy_file=$(mktemp)
cat >"$policy_file" <<EOF
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": ["s3:GetBucketLocation", "s3:GetBucketVersioning", "s3:ListBucket", "s3:ListBucketVersions"],
      "Resource": ["arn:aws:s3:::$S3_BUCKET"]
    },
    {
      "Effect": "Allow",
      "Action": ["s3:PutObject", "s3:GetObject", "s3:GetObjectVersion"],
      "Resource": ["arn:aws:s3:::$S3_BUCKET/*"]
    },
    {
      "Effect": "Deny",
      "Action": [
        "s3:DeleteObject", "s3:DeleteObjectVersion", "s3:DeleteBucket",
        "s3:PutBucketVersioning", "s3:PutLifecycleConfiguration", "s3:PutBucketPolicy"
      ],
      "Resource": ["arn:aws:s3:::$S3_BUCKET", "arn:aws:s3:::$S3_BUCKET/*"]
    }
  ]
}
EOF
mc admin policy create local "$POLICY" "$policy_file" >/dev/null
rm -f "$policy_file"

mc admin user add local "$S3_ACCESS_KEY" "$S3_SECRET_KEY" >/dev/null
# attach é idempotente nesta versão do mc (imagem fixada por digest): reanexar não dá erro.
mc admin policy attach local "$POLICY" --user "$S3_ACCESS_KEY" >/dev/null

echo "minio-init: bucket $S3_BUCKET versionado; usuário $S3_ACCESS_KEY com a política $POLICY (sem exclusão)"
