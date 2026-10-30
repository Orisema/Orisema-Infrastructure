# Orisema Infrastructure

> Uma ideia nasce. Muitas mãos a fazem crescer.

Projeto comunitário para simplificar a implantação, atualização, diagnóstico e recuperação de aplicações em servidores Linux.

**Status:** protótipo experimental com CLI funcional para Podman; ainda não está pronto para produção.

**Aviso:** o Orisema Infrastructure está em desenvolvimento experimental e não
é recomendado para ambientes de produção.

## Objetivos

- Implantação declarativa e verificável de aplicações em contêineres.
- Podman como primeiro runtime, com arquitetura extensível.
- Plano de mudanças antes da execução e segurança por padrão.
- Evolução para backup, restauração, rollback e observabilidade.
- Software livre, documentação acessível e governança comunitária.

## Documentação

- [Manifesto](MANIFESTO.md)
- [Arquitetura proposta](docs/ARCHITECTURE.md)
- [Roadmap](ROADMAP.md)
- [Como contribuir](CONTRIBUTING.md)
- [Governança](GOVERNANCE.md)
- [Segurança](SECURITY.md)
- [Changelog](CHANGELOG.md)
- [Limitações conhecidas da Alpha 1](docs/KNOWN_LIMITATIONS.md)
- [Contrato de manifesto da Alpha 2](docs/MANIFEST-V1ALPHA2.md)
- [Laboratório de update e rollback da Alpha 3](docs/CONTROLLED-UPDATE-LAB.md)
- [Backup e restauração de configuração](docs/BACKUP-RESTORE-DESIGN.md)
- [Projeto de recuperação confiável da Alpha 4](docs/ALPHA4-RECOVERY-DESIGN.md)
- [Plano somente leitura de update da Alpha 5](docs/ALPHA5-UPDATE-PLAN.md)
- [Instalação e verificação de releases](docs/INSTALLATION.md)
- [Quick start em host Linux descartável](docs/QUICKSTART.md)
- [Compatibilidade e comandos suportados](docs/COMPATIBILITY.md)

## Versão do binário

Builds locais não exigem metadados externos e se identificam como builds de
desenvolvimento:

```sh
go build -o orisema ./cmd/orisema
./orisema version
```

O [pré-release v0.1.0-alpha.4](https://github.com/Orisema/Orisema-Infrastructure/releases/tag/v0.1.0-alpha.4)
distribui binários para Linux AMD64 e Linux ARM64. Podman
em Linux é o único runtime suportado inicialmente; Docker, Windows e macOS não
têm suporte oficial nesta etapa. Consulte as diferenças entre alvos compilados
e plataformas efetivamente testadas em
[Instalação](docs/INSTALLATION.md).

## Quick start

Em um host Linux descartável com Podman, siga o
[quick start revisável](docs/QUICKSTART.md). Ele começa por `validate` e `plan`,
exige confirmação explícita antes de criar o contêiner e termina com limpeza
manual baseada nos labels de ownership. Não execute o laboratório em um host de
produção.

## Licença

Distribuído sob a [Apache License 2.0](LICENSE). Contribuições devem respeitar a licença e os direitos de terceiros.

A Orisema é independente da Mix Fiscal e não incorpora materiais internos ou proprietários de terceiros.
