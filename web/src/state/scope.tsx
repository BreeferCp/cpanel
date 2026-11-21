import { useParams } from "react-router-dom";

// Hosting pages are mounted twice: /hosting/* for an account holder acting on
// their own account ("me"), and /accounts/:aid/* for an administrator managing
// a customer. Pages only ever call useAccountId().
export function useAccountId(): string {
  const { aid } = useParams();
  return aid ?? "me";
}

export function useBase(): string {
  const { aid } = useParams();
  return aid ? `/accounts/${aid}` : "/hosting";
}
