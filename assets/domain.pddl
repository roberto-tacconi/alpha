(define (domain alpha)
    (:requirements :strips :adl :typing :equality :negative-preconditions :disjunctive-preconditions :existential-preconditions :universal-preconditions :quantified-preconditions :conditional-effects :derived-predicates :action-costs
    )

    (:types
        capability service resource - object
        digital analog - resource
        root child - capability
    )

    (:predicates
        ;; Capability:
        (capability-can-inherit-criticality ?cap - capability)
        (context-critical ?cap - root)
        (inherits-criticality ?consumed - capability) ;; Derived
        (is-part-of ?child - child ?root - root)

        (is-available ?cap - capability) ;; Derived 

        (can-be-provided ?provided - capability ?provider - service)
        (root-is-provided-by ?provided - root ?provider - service)
        (is-provided-by ?provided - capability ?provider - service) ;; Derived

        ;; Service:
        (provider-can-be-critical ?srv - service)
        (provider-can-inherit-criticality ?srv - service)
        (is-running ?srv - service) ;; derived
        (requirements-satisfied ?srv - service) ;; derived
        (transitively-requires ?consumer - service ?consumed - capability)

        (can-be-hosted ?srv - service ?resource - resource)
        (is-hosted-on ?srv - service ?resource - resource)

        ;; Resource:
        (host-can-be-critical ?host - digital)
        (critical-host ?host - digital) ;; derived

        ;; Digital
        (is-powered-on ?host - digital)
        (is-quarantined ?host - digital)
        (is-compromised ?host - digital)

        (has-clean-image ?host - digital)

        ;; Requests:
        (threat-contained ?host - digital)
        (threat-removed ?host - digital)
        (host-recovered ?host - digital)
    )

    (:functions
        (total-cost)

        (cost-switch ?cap - root ?to - service)
        (cost-failover ?srv - service ?a - resource)

        (penalty-active-threat ?host - digital)
        (penalty-existing-threat ?host - digital)
        (penalty-unrecovered-host ?host - digital)

        (cost-isolate ?host - digital)
        (cost-reconnect ?host - digital)
        (cost-reconnect-recovery ?host - digital)
        (cost-reconnect-recovery-unsafe ?host - digital)

        (cost-shutdown ?host - digital)
        (cost-power-on ?host - digital)
        (cost-power-on-recovery ?host - digital)
        (cost-power-on-recovery-unsafe ?host - digital)

        (cost-rollback ?host - digital)
        (cost-rollback-unsafe ?host - digital)
    )

    (:derived
        (inherits-criticality ?consumed - capability)
        (and
            (capability-can-inherit-criticality ?consumed)
            (exists
                (?consumer - service ?produced - root)
                (and
                    (context-critical ?produced)
                    (root-is-provided-by ?produced ?consumer)
                    (transitively-requires ?consumer ?consumed)
                )
            )
        )
    )

    (:derived
        (is-available ?cap - capability)
        (exists
            (?srv - service)
            (and
                (is-provided-by ?cap ?srv)
                (is-running ?srv)
            )
        )
    )

    (:derived
        (is-provided-by ?provided - capability ?provider - service)
        (and
            (can-be-provided ?provided ?provider)
            (or
                (root-is-provided-by ?provided ?provider)

                (exists
                    (?parent - root)
                    (and
                        (is-part-of ?provided ?parent)
                        (root-is-provided-by ?parent ?provider)
                    )
                )
            )
        )
    )

    (:derived
        (is-running ?srv - service)
        (or
            (exists
                (?host - digital)
                (and
                    (is-hosted-on ?srv ?host)
                    (is-powered-on ?host)
                    (not (is-quarantined ?host))
                    (not (is-compromised ?host))
                )
            )
            (exists
                (?host - analog)
                (is-hosted-on ?srv ?host)
            )
        )
    )

    (:derived
        (requirements-satisfied ?srv - service)
        (forall
            (?consumed - capability)
            (or
                (not (transitively-requires ?srv ?consumed))
                (is-available ?consumed)
            )
        )
    )

    (:derived
        (critical-host ?host - digital)
        (and
            (host-can-be-critical ?host)
            (exists
                (?srv - service)
                (and
                    (is-hosted-on ?srv ?host)

                    (or
                        (provider-can-be-critical ?srv)
                        (provider-can-inherit-criticality ?srv)
                    )

                    (or
                        (exists
                            (?cap - root)
                            (and
                                (context-critical ?cap)
                                (root-is-provided-by ?cap ?srv)
                            )
                        )
                        (exists
                            (?cap - capability)
                            (and
                                (inherits-criticality ?cap)
                                (is-provided-by ?cap ?srv)
                            )
                        )
                    )
                )
            )
        )
    )

    ;; Requests

    (:action abandon-threat-containment
        :parameters (?host - digital)
        :precondition (and
            (is-compromised ?host)
            (is-powered-on ?host)
            (not (is-quarantined ?host))

            (not (threat-contained ?host))
        )
        :effect (and
            (threat-contained ?host)

            (increase
                (total-cost)
                (penalty-active-threat ?host))
        )
    )

    (:action abandon-threat-removal
        :parameters (?host - digital)
        :precondition (and
            (threat-contained ?host)
            (is-compromised ?host)
            (not (threat-removed ?host))
        )
        :effect (and
            (threat-removed ?host)
            (increase
                (total-cost)
                (penalty-existing-threat ?host))
        )
    )

    (:action abandon-digital-recovery
        :parameters (?host - digital)
        :precondition (and
            (threat-removed ?host)
            (not (host-recovered ?host))
        )
        :effect (and
            (host-recovered ?host)
            (increase
                (total-cost)
                (penalty-unrecovered-host ?host))
        )
    )

    ;; Dynamic reconfiguration

    (:action switch-provider
        :parameters (?from - service ?to - service ?cap - root)
        :precondition (and
            (not (= ?from ?to))
            (root-is-provided-by ?cap ?from)

            (can-be-provided ?cap ?to)
            (is-running ?to)
            (requirements-satisfied ?to)
        )
        :effect (and
            (not (root-is-provided-by ?cap ?from))
            (root-is-provided-by ?cap ?to)

            (increase
                (total-cost)
                (cost-switch ?cap ?to))
        )
    )

    (:action failover_to-analog
        :parameters (?from - resource ?to - analog ?srv - service)
        :precondition (and
            (not (= ?from ?to))
            (is-hosted-on ?srv ?from)

            (can-be-hosted ?srv ?to)
        )
        :effect (and
            (not (is-hosted-on ?srv ?from))
            (is-hosted-on ?srv ?to)

            (increase
                (total-cost)
                (cost-failover ?srv ?to))
        )
    )

    (:action failover_to-digital
        :parameters (?from - resource ?to - digital ?srv - service)
        :precondition (and
            (not (= ?from ?to))
            (is-hosted-on ?srv ?from)

            (can-be-hosted ?srv ?to)
            (is-powered-on ?to)
            (not (is-quarantined ?to))
            (not (is-compromised ?to))
        )
        :effect (and
            (not (is-hosted-on ?srv ?from))
            (is-hosted-on ?srv ?to)

            (increase
                (total-cost)
                (cost-failover ?srv ?to))
        )
    )

    ;; Containment

    (:action isolate
        :parameters (?host - digital)
        :precondition (and
            (not (is-quarantined ?host))
            (not (critical-host ?host))
        )
        :effect (and
            (is-quarantined ?host)
            (threat-contained ?host)

            (increase (total-cost) (cost-isolate ?host))
        )
    )

    (:action reconnect
        :parameters (?host - digital)
        :precondition (and
            (is-quarantined ?host)
            (not (is-powered-on ?host))
        )
        :effect (and
            (not (is-quarantined ?host))

            (increase (total-cost) (cost-reconnect ?host))
        )
    )

    (:action reconnect_recovery
        :parameters (?host - digital)
        :precondition (and
            (is-quarantined ?host)
            (is-powered-on ?host)
            (not (is-compromised ?host))

            (forall
                (?other - digital)
                (or
                    (not (is-powered-on ?other))
                    (is-quarantined ?other)
                    (not (is-compromised ?other))
                )
            )
        )
        :effect (and
            (not (is-quarantined ?host))
            (host-recovered ?host)

            (increase
                (total-cost)
                (cost-reconnect-recovery ?host))
        )
    )

    (:action reconnect_recovery_unsafe
        :parameters (?host - digital)
        :precondition (and
            (is-quarantined ?host)
            (is-powered-on ?host)
            (not (is-compromised ?host))

            (exists
                (?other - digital)
                (and
                    (is-powered-on ?other)
                    (not (is-quarantined ?other))
                    (is-compromised ?other)
                )
            )
        )
        :effect (and
            (not (is-quarantined ?host))
            (host-recovered ?host)

            (increase
                (total-cost)
                (cost-reconnect-recovery-unsafe ?host))
        )
    )

    (:action shutdown
        :parameters (?host - digital)
        :precondition (and
            (is-powered-on ?host)
            (not (critical-host ?host))
        )
        :effect (and
            (not (is-powered-on ?host))
            (threat-contained ?host)

            (increase (total-cost) (cost-shutdown ?host))
        )
    )

    (:action power-on
        :parameters (?host - digital)
        :precondition (and
            (not (is-powered-on ?host))
            (is-quarantined ?host)
        )
        :effect (and
            (is-powered-on ?host)

            (increase (total-cost) (cost-power-on ?host))
        )
    )

    (:action power-on_recovery
        :parameters (?host - digital)
        :precondition (and
            (not (is-powered-on ?host))
            (not (is-quarantined ?host))
            (not (is-compromised ?host))

            (forall
                (?other - digital)
                (or
                    (not (is-powered-on ?other))
                    (is-quarantined ?other)
                    (not (is-compromised ?other))
                )
            )
        )
        :effect (and
            (is-powered-on ?host)
            (host-recovered ?host)

            (increase
                (total-cost)
                (cost-power-on-recovery ?host))
        )
    )

    (:action power-on_recovery_unsafe
        :parameters (?host - digital)
        :precondition (and
            (not (is-powered-on ?host))
            (not (is-quarantined ?host))
            (not (is-compromised ?host))

            (exists
                (?other - digital)
                (and
                    (is-powered-on ?other)
                    (not (is-quarantined ?other))
                    (is-compromised ?other)
                )
            )
        )
        :effect (and
            (is-powered-on ?host)
            (host-recovered ?host)

            (increase
                (total-cost)
                (cost-power-on-recovery-unsafe ?host))
        )
    )

    ;; Eradication 

    (:action rollback-image
        :parameters (?host - digital)
        :precondition (and
            (is-quarantined ?host)

            (has-clean-image ?host)
            (not (critical-host ?host))

            (forall
                (?other - digital)
                (or
                    (not (is-powered-on ?other))
                    (is-quarantined ?other)
                    (not (is-compromised ?other))
                )
            )

            (forall
                (?cap - capability)
                (or
                    (not
                        (or
                            (context-critical ?cap)
                            (and
                                (capability-can-inherit-criticality ?cap)
                                (inherits-criticality ?cap)
                            )
                        )
                    )
                    (is-available ?cap)
                )
            )
        )
        :effect (and
            (not (is-compromised ?host))

            (threat-contained ?host)
            (threat-removed ?host)

            (increase
                (total-cost)
                (cost-rollback ?host))
        )
    )

    (:action rollback-image_unsafe
        :parameters (?host - digital)
        :precondition (and
            (is-quarantined ?host)

            (has-clean-image ?host)
            (not (critical-host ?host))

            (or
                (exists
                    (?other - digital)
                    (and
                        (is-powered-on ?other)
                        (not (is-quarantined ?other))
                        (is-compromised ?other)
                    )
                )

                (exists
                    (?cap - capability)
                    (and
                        (or
                            (context-critical ?cap)

                            (and
                                (capability-can-inherit-criticality ?cap)
                                (inherits-criticality ?cap)
                            )
                        )
                        (not (is-available ?cap))
                    )
                )
            )
        )
        :effect (and
            (not (is-compromised ?host))

            (threat-contained ?host)
            (threat-removed ?host)

            (increase
                (total-cost)
                (cost-rollback-unsafe ?host))
        )
    )
)