import { generateAutonomousPlan, type AutonomousPlan } from "@/api/plans";
export class PlanningStore{async generate(input:{idea:string;purpose:string;targetUser:string;environment:string;constraints:string[]}):Promise<AutonomousPlan>{return generateAutonomousPlan(input)}}
export const planningStore=new PlanningStore();
