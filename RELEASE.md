# Relase Process

This is a summary of the steps to get a change into production.

1. Merge sqs PR into main (v25.x) once approved
2. Create a tag
3. Create [infrastructure](https://github.com/osmosis-labs/infrastructure) PR for updating stage configs config.json.j2, sqs.yaml, if needed, as well as versions.yaml
4. Get infrastrucuture PR approved and merged into main
5. Deploy to stage (automated and triggered by step 4)
6. Test & request QA
7. Repeat step 3 and 4 for prod
8. Manually perform prod deployment