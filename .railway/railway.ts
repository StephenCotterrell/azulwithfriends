import { defineRailway, github, project, service } from "railway/iac";

export const partial = "azulwithfriends";

export default defineRailway(() => {
  const app = service("azulwithfriends", {
    source: github("StephenCotterrell/azulwithfriends", { branch: "master", checkSuites: false }),
    build: { buildEnvironment: "V3", builder: "DOCKERFILE", dockerfilePath: "Dockerfile" },
    deploy: { sleepApplication: false },
    preDeploy: "/app/server migrate",
    start: "/app/server",
    healthcheck: "/healthz",
    healthcheckTimeout: 60,
    replicas: { sfo: 1 },
  });

  return project("ideal-purpose", {
    variables: { managed: false },
    resources: [app],
  });
});
